// Chaos API — composition root.
// main.go only wires dependencies (constructor injection) and owns process
// lifecycle: config → logger → db+migrations → cache → bus/workers → router →
// graceful shutdown. No business logic lives here.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"

	_ "github.com/chaosapp/backend/docs/swagger" // generated OpenAPI spec
	"github.com/chaosapp/backend/internal/ai"
	fbauth "github.com/chaosapp/backend/internal/auth/firebase"
	"github.com/chaosapp/backend/internal/auth/jwt"
	"github.com/chaosapp/backend/internal/cache"
	"github.com/chaosapp/backend/internal/config"
	"github.com/chaosapp/backend/internal/database"
	authhandler "github.com/chaosapp/backend/internal/domain/auth/handler"
	authrepo "github.com/chaosapp/backend/internal/domain/auth/repository"
	authservice "github.com/chaosapp/backend/internal/domain/auth/service"
	convhandler "github.com/chaosapp/backend/internal/domain/conversation/handler"
	convrepo "github.com/chaosapp/backend/internal/domain/conversation/repository"
	convservice "github.com/chaosapp/backend/internal/domain/conversation/service"
	"github.com/chaosapp/backend/internal/domain/device"
	grouphandler "github.com/chaosapp/backend/internal/domain/group/handler"
	grouprepo "github.com/chaosapp/backend/internal/domain/group/repository"
	groupservice "github.com/chaosapp/backend/internal/domain/group/service"
	"github.com/chaosapp/backend/internal/domain/link"
	mediahandler "github.com/chaosapp/backend/internal/domain/media/handler"
	profilehandler "github.com/chaosapp/backend/internal/domain/profile/handler"
	profilerepo "github.com/chaosapp/backend/internal/domain/profile/repository"
	profileservice "github.com/chaosapp/backend/internal/domain/profile/service"
	userhandler "github.com/chaosapp/backend/internal/domain/user/handler"
	userrepo "github.com/chaosapp/backend/internal/domain/user/repository"
	userservice "github.com/chaosapp/backend/internal/domain/user/service"
	"github.com/chaosapp/backend/internal/email"
	"github.com/chaosapp/backend/internal/events"
	"github.com/chaosapp/backend/internal/logger"
	"github.com/chaosapp/backend/internal/middleware"
	"github.com/chaosapp/backend/internal/push"
	"github.com/chaosapp/backend/internal/realtime"
	"github.com/chaosapp/backend/internal/routes"
	"github.com/chaosapp/backend/internal/storage"
	"github.com/chaosapp/backend/internal/worker"
)

// @title           Chaos API
// @version         1.0
// @description     Group chat with an AI that actually decides things. Clean architecture, JWT auth with refresh rotation.
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// @description     Type "Bearer" followed by a space and the access token.
func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		panic("config: " + err.Error())
	}
	log, err := logger.New(cfg.App.Env)
	if err != nil {
		panic("logger: " + err.Error())
	}
	defer func() { _ = log.Sync() }()

	if err := run(cfg, log); err != nil {
		log.Fatal("server exited", zap.Error(err))
	}
}

func run(cfg *config.Config, log *zap.Logger) error {
	// --- infrastructure ---
	db, err := database.Connect(cfg.Database, log)
	if err != nil {
		return err
	}
	if err := database.Migrate(cfg.Database.URL, "migrations", log); err != nil {
		return err
	}

	var store cache.Store
	if redis, err := cache.NewRedis(cfg.Redis); err != nil {
		log.Warn("redis unavailable — caching and rate limiting disabled", zap.Error(err))
		store = cache.Noop{}
	} else {
		store = redis
	}

	sender := buildEmailSender(cfg, log)
	bus := worker.New(log, 256)
	registerEmailWorkers(bus, sender, cfg)
	bus.Start(4)
	defer bus.Stop()

	// --- domain wiring (constructor injection, interfaces everywhere) ---
	tokens := jwt.NewManager(cfg.JWT)

	usersRepo := userrepo.NewGorm(db)
	usersService := userservice.New(usersRepo, store, log)
	usersHandler := userhandler.New(usersService)

	devicesRepo := device.NewGorm(db)
	devicesHandler := device.NewHandler(devicesRepo)

	firebaseVerifier, err := fbauth.New(
		context.Background(), cfg.Push.CredentialsFile, cfg.Push.ProjectID)
	if err != nil {
		log.Warn("social sign-in disabled: firebase init failed", zap.Error(err))
		firebaseVerifier = fbauth.Disabled{}
	}
	switch {
	case !firebaseVerifier.Enabled():
		log.Info("social sign-in disabled: set FIREBASE_PROJECT_ID to enable")
	case cfg.Push.CredentialsFile != "":
		log.Info("social sign-in enabled (Apple/Google via Firebase)")
	default:
		// Worth saying out loud: everything a person does works, but deleting
		// an account will leave its Firebase identity behind.
		log.Info("social sign-in enabled, verify-only",
			zap.String("project", cfg.Push.ProjectID),
			zap.String("note", "set FIREBASE_CREDENTIALS_FILE to also delete identities"))
	}

	tokenRepo := authrepo.NewGorm(db)
	authService := authservice.New(usersRepo, tokenRepo, devicesRepo,
		firebaseVerifier, tokens, bus, log, cfg.JWT.AccessTTL)
	authHandler := authhandler.New(authService)

	mediaStore, err := buildStorage(cfg)
	if err != nil {
		return err
	}
	mediaHandler := mediahandler.New(mediaStore)
	linkHandler := link.NewHandler(link.New(store, log))

	// Chaos itself. Missing credentials degrade to a disabled client so the
	// rest of the server is unaffected — the app stays a working group chat,
	// it just stops answering.
	aiClient := ai.NewAzure(ai.Config{
		Endpoint:       cfg.AI.Endpoint,
		APIKey:         cfg.AI.APIKey,
		ChatDeployment: cfg.AI.ChatDeployment,
		APIVersion:     cfg.AI.APIVersion,
	})
	if aiClient.Enabled() {
		log.Info("Chaos enabled", zap.String("chat", cfg.AI.ChatDeployment))
	} else {
		log.Info("Chaos disabled: no AZURE_OPENAI_* configuration")
	}

	hub := realtime.NewHub(log)
	notifier := push.NewNotifier(devicesRepo, buildPusher(cfg, log), log)

	conversationsRepo := convrepo.NewGorm(db)

	// The two domain modules each read a slice of the other, and neither
	// imports the other: conversation declares a Facts port that the profile
	// service satisfies, profile declares a History port that the conversation
	// repository satisfies, and the wiring happens here.
	profilesRepo := profilerepo.NewGorm(db)
	profilesService := profileservice.New(profilesRepo, usersRepo,
		conversationsRepo, aiClient, store, log, cfg.AI.PerHourLimit)
	profilesHandler := profilehandler.New(profilesService)

	// Conversations and groups read a slice of each other, both through ports
	// each declares itself: a conversation needs the group's standing memory
	// for every Chaos turn, and a group needs the conversations' transcripts to
	// answer a question across them. The cycle is broken by constructing the
	// group service with a lazily-resolved threads port.
	chaos := convservice.NewChaos(aiClient, store, log, cfg.AI.PerHourLimit)
	threads := &lazyThreads{}
	people := &lazyPeople{}

	groupsRepo := grouprepo.NewGorm(db)
	groupsService := groupservice.New(groupsRepo, usersRepo, people, threads,
		aiClient, store, log, cfg.AI.PerHourLimit)
	groupsHandler := grouphandler.New(groupsService)

	conversations := convservice.New(conversationsRepo, usersRepo, chaos,
		profilesService, groupsService, hub, notifier, log, cfg.Storage.PublicBaseURL)
	threads.inner = conversations
	people.inner = conversations
	conversationsHandler := convhandler.New(conversations, conversations)

	// --- HTTP ---
	if cfg.App.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(
		middleware.RequestID(),
		middleware.Recovery(log),
		middleware.Logging(log),
		middleware.SecureHeaders(),
		middleware.CORS(cfg.HTTP.AllowedOrigins),
	)
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))
	r.GET("/api/v1/ws", realtime.Handler(hub, tokens, conversationsRepo, log))
	routes.Register(r, routes.Deps{
		Tokens:        tokens,
		Cache:         store,
		RateLimitRPM:  cfg.HTTP.RateLimitRPM,
		Auth:          authHandler,
		Users:         usersHandler,
		Conversations: conversationsHandler,
		Groups:        groupsHandler,
		Profiles:      profilesHandler,
		Media:         mediaHandler,
		Links:         linkHandler,
		Devices:       devicesHandler,
		UploadDir:     cfg.Storage.LocalDir,
	})

	return serve(r, cfg, log)
}

// lazyThreads and lazyPeople break the construction cycle between the group
// and conversation services. Both are wired immediately after both services
// exist, so `inner` is never nil by the time a request can reach them — the
// alternative is a setter on one of the services, which would leave a
// half-built object reachable from anywhere.
type lazyThreads struct {
	inner convservice.ConversationService
}

func (l *lazyThreads) ForGroup(ctx context.Context, groupID uuid.UUID, limit int) ([]groupservice.Thread, error) {
	return l.inner.ForGroup(ctx, groupID, limit)
}

type lazyPeople struct {
	inner convservice.ConversationService
}

func (l *lazyPeople) Placeholder(ctx context.Context, name string) (uuid.UUID, error) {
	return l.inner.Placeholder(ctx, name)
}

// serve runs the HTTP server with graceful shutdown on SIGINT/SIGTERM.
func serve(r *gin.Engine, cfg *config.Config, log *zap.Logger) error {
	srv := &http.Server{
		Addr:         ":" + cfg.HTTP.Port,
		Handler:      r,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", zap.String("port", cfg.HTTP.Port), zap.String("env", cfg.App.Env))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case <-quit:
		log.Info("shutting down gracefully")
		ctx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

// buildStorage picks the media driver from config (local default, s3/R2).
func buildStorage(cfg *config.Config) (storage.Storage, error) {
	if cfg.Storage.Driver == "s3" {
		return storage.NewS3(context.Background(), cfg.Storage)
	}
	return storage.NewLocal(cfg.Storage.LocalDir, cfg.Storage.PublicBaseURL)
}

// buildPusher returns an FCM sender when credentials exist, else a no-op.
func buildPusher(cfg *config.Config, log *zap.Logger) push.Sender {
	fcm, err := push.NewFCM(context.Background(), cfg.Push.CredentialsFile, log)
	if err != nil {
		log.Warn("push disabled: firebase init failed", zap.Error(err))
		return push.Disabled{}
	}
	if fcm == nil {
		log.Info("push disabled: no FIREBASE_CREDENTIALS_FILE configured")
		return push.Disabled{}
	}
	log.Info("push enabled (FCM)")
	return fcm
}

func buildEmailSender(cfg *config.Config, log *zap.Logger) email.Sender {
	if cfg.Email.Driver == "smtp" {
		return email.NewSMTPSender(cfg.Email)
	}
	return email.NewLogSender(log)
}

// registerEmailWorkers subscribes mail side effects to domain events.
func registerEmailWorkers(bus *worker.Pool, sender email.Sender, cfg *config.Config) {
	base := cfg.Storage.PublicBaseURL
	bus.Subscribe(events.UserRegistered, func(ctx context.Context, e events.Event) error {
		return sender.Send(ctx, email.Message{
			To:      e.Payload["email"],
			Subject: "Welcome to Chaos — verify your email",
			Body: "Hi " + e.Payload["name"] + ",\n\nVerify your email:\n" +
				base + "/verify-email?token=" + e.Payload["token"] + "\n",
		})
	})
	bus.Subscribe(events.PasswordResetRequested, func(ctx context.Context, e events.Event) error {
		return sender.Send(ctx, email.Message{
			To:      e.Payload["email"],
			Subject: "Reset your Chaos password",
			Body: "Hi " + e.Payload["name"] + ",\n\nReset your password (valid 30 minutes):\n" +
				base + "/reset-password?token=" + e.Payload["token"] + "\n",
		})
	})
}
