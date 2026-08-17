// Package config loads typed application configuration with Viper.
// Sources (highest wins): environment variables → .env file → defaults.
// Environments: development | staging | production (APP_ENV).
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Database DatabaseConfig
	Redis    RedisConfig
	JWT      JWTConfig
	Email    EmailConfig
	Storage  StorageConfig
	Push     PushConfig
	AI       AIConfig
}

// AIConfig holds Azure OpenAI credentials. Leave Endpoint or APIKey empty and
// Chaos switches itself off rather than erroring at runtime — the app still
// works as a group chat, it just stops answering.
type AIConfig struct {
	Endpoint       string
	APIKey         string
	ChatDeployment string
	APIVersion     string
	// PerHourLimit caps model calls per user per hour — these cost money, and
	// every sent message can trigger one.
	PerHourLimit int
}

type PushConfig struct {
	// Firebase service-account JSON; empty disables push entirely.
	CredentialsFile string

	// Firebase project ID. Enough on its own to verify Apple/Google ID tokens,
	// because that only needs Google's public certificates — so social sign-in
	// can be switched on without a private key anywhere near the repo. Push
	// still needs the service account.
	ProjectID string
}

type AppConfig struct {
	Name string
	Env  string // development | staging | production
}

type HTTPConfig struct {
	Port            string
	AllowedOrigins  []string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	RateLimitRPM    int // requests/min per client
}

type DatabaseConfig struct {
	URL             string // postgres://user:pass@host:5432/db?sslmode=disable
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	LogQueries      bool
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	Issuer        string
}

type EmailConfig struct {
	Driver   string // log | smtp
	From     string
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
}

type StorageConfig struct {
	Driver        string // local | s3
	LocalDir      string
	PublicBaseURL string
	S3Bucket      string
	S3Region      string
	S3Endpoint    string // set for R2/MinIO
}

func (c AppConfig) IsProduction() bool { return c.Env == "production" }

// Load reads configuration. envFile may be empty (env vars only).
func Load(envFile string) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	if envFile != "" {
		v.SetConfigFile(envFile)
		v.SetConfigType("env")
		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok &&
				!strings.Contains(err.Error(), "no such file") {
				return nil, fmt.Errorf("read %s: %w", envFile, err)
			}
		}
	}
	v.AutomaticEnv()

	cfg := &Config{
		App: AppConfig{
			Name: v.GetString("APP_NAME"),
			Env:  v.GetString("APP_ENV"),
		},
		HTTP: HTTPConfig{
			Port:            v.GetString("PORT"),
			AllowedOrigins:  strings.Split(v.GetString("ALLOWED_ORIGINS"), ","),
			ReadTimeout:     v.GetDuration("HTTP_READ_TIMEOUT"),
			WriteTimeout:    v.GetDuration("HTTP_WRITE_TIMEOUT"),
			ShutdownTimeout: v.GetDuration("HTTP_SHUTDOWN_TIMEOUT"),
			RateLimitRPM:    v.GetInt("RATE_LIMIT_RPM"),
		},
		Database: DatabaseConfig{
			URL:             v.GetString("DATABASE_URL"),
			MaxOpenConns:    v.GetInt("DB_MAX_OPEN_CONNS"),
			MaxIdleConns:    v.GetInt("DB_MAX_IDLE_CONNS"),
			ConnMaxLifetime: v.GetDuration("DB_CONN_MAX_LIFETIME"),
			LogQueries:      v.GetBool("DB_LOG_QUERIES"),
		},
		Redis: RedisConfig{
			Addr:     v.GetString("REDIS_ADDR"),
			Password: v.GetString("REDIS_PASSWORD"),
			DB:       v.GetInt("REDIS_DB"),
		},
		JWT: JWTConfig{
			AccessSecret:  v.GetString("JWT_ACCESS_SECRET"),
			RefreshSecret: v.GetString("JWT_REFRESH_SECRET"),
			AccessTTL:     v.GetDuration("JWT_ACCESS_TTL"),
			RefreshTTL:    v.GetDuration("JWT_REFRESH_TTL"),
			Issuer:        v.GetString("JWT_ISSUER"),
		},
		Email: EmailConfig{
			Driver:   v.GetString("EMAIL_DRIVER"),
			From:     v.GetString("EMAIL_FROM"),
			SMTPHost: v.GetString("SMTP_HOST"),
			SMTPPort: v.GetInt("SMTP_PORT"),
			SMTPUser: v.GetString("SMTP_USER"),
			SMTPPass: v.GetString("SMTP_PASS"),
		},
		AI: AIConfig{
			Endpoint:       v.GetString("AZURE_OPENAI_ENDPOINT"),
			APIKey:         v.GetString("AZURE_OPENAI_API_KEY"),
			ChatDeployment: v.GetString("AZURE_OPENAI_CHAT_DEPLOYMENT"),
			APIVersion:     v.GetString("AZURE_OPENAI_API_VERSION"),
			PerHourLimit:   v.GetInt("AI_PER_HOUR_LIMIT"),
		},
		Push: PushConfig{
			CredentialsFile: v.GetString("FIREBASE_CREDENTIALS_FILE"),
			ProjectID:       v.GetString("FIREBASE_PROJECT_ID"),
		},
		Storage: StorageConfig{
			Driver:        v.GetString("STORAGE_DRIVER"),
			LocalDir:      v.GetString("STORAGE_LOCAL_DIR"),
			PublicBaseURL: v.GetString("STORAGE_PUBLIC_BASE_URL"),
			S3Bucket:      v.GetString("S3_BUCKET"),
			S3Region:      v.GetString("S3_REGION"),
			S3Endpoint:    v.GetString("S3_ENDPOINT"),
		},
	}
	return cfg, cfg.validate()
}

func (c *Config) validate() error {
	if c.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.JWT.AccessSecret == "" || c.JWT.RefreshSecret == "" {
		return fmt.Errorf("JWT_ACCESS_SECRET and JWT_REFRESH_SECRET are required")
	}
	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("APP_NAME", "chaos")
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("PORT", "8080")
	v.SetDefault("ALLOWED_ORIGINS", "*")
	// Chaos can answer on any message, so the ceiling has to allow a real
	// conversation while still bounding a runaway client.
	v.SetDefault("AI_PER_HOUR_LIMIT", 120)
	v.SetDefault("AZURE_OPENAI_API_VERSION", "2025-04-01-preview")
	v.SetDefault("HTTP_READ_TIMEOUT", "15s")
	v.SetDefault("HTTP_WRITE_TIMEOUT", "30s")
	v.SetDefault("HTTP_SHUTDOWN_TIMEOUT", "10s")
	v.SetDefault("RATE_LIMIT_RPM", 120)
	v.SetDefault("DB_MAX_OPEN_CONNS", 25)
	v.SetDefault("DB_MAX_IDLE_CONNS", 10)
	v.SetDefault("DB_CONN_MAX_LIFETIME", "30m")
	v.SetDefault("DB_LOG_QUERIES", false)
	v.SetDefault("REDIS_ADDR", "localhost:6379")
	v.SetDefault("REDIS_DB", 0)
	v.SetDefault("JWT_ACCESS_TTL", "15m")
	v.SetDefault("JWT_REFRESH_TTL", "720h")
	v.SetDefault("JWT_ISSUER", "chaos")
	v.SetDefault("EMAIL_DRIVER", "log")
	v.SetDefault("EMAIL_FROM", "hello@chaos.app")
	v.SetDefault("SMTP_PORT", 587)
	v.SetDefault("STORAGE_DRIVER", "local")
	v.SetDefault("STORAGE_LOCAL_DIR", "./uploads")
	v.SetDefault("STORAGE_PUBLIC_BASE_URL", "http://localhost:8080")
}
