// Package routes wires handlers to versioned URL groups. Adding /api/v2 later
// means adding one function here — handlers stay untouched.
package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/chaosapp/backend/internal/auth/jwt"
	"github.com/chaosapp/backend/internal/cache"
	"github.com/chaosapp/backend/internal/common/constants"
	authhandler "github.com/chaosapp/backend/internal/domain/auth/handler"
	convhandler "github.com/chaosapp/backend/internal/domain/conversation/handler"
	"github.com/chaosapp/backend/internal/domain/device"
	grouphandler "github.com/chaosapp/backend/internal/domain/group/handler"
	"github.com/chaosapp/backend/internal/domain/link"
	mediahandler "github.com/chaosapp/backend/internal/domain/media/handler"
	profilehandler "github.com/chaosapp/backend/internal/domain/profile/handler"
	userhandler "github.com/chaosapp/backend/internal/domain/user/handler"
	"github.com/chaosapp/backend/internal/middleware"
)

// Deps carries everything route registration needs — one struct so the
// signature never churns as modules are added.
type Deps struct {
	Tokens        jwt.Manager
	Cache         cache.Store
	RateLimitRPM  int
	Auth          *authhandler.Handler
	Users         *userhandler.Handler
	Conversations *convhandler.Handler
	Groups        *grouphandler.Handler
	Profiles      *profilehandler.Handler
	Devices       *device.Handler
	Media         *mediahandler.Handler
	Links         *link.Handler
	UploadDir     string
}

// Register mounts /api/v1.
func Register(r *gin.Engine, d Deps) {
	// Local-storage files (no-op path when the S3 driver is active).
	r.Static("/uploads", d.UploadDir)

	v1 := r.Group("/api/v1")
	v1.Use(middleware.RateLimit(d.Cache, d.RateLimitRPM))

	// Public auth endpoints.
	auth := v1.Group("/auth")
	{
		auth.POST("/register", d.Auth.Register)
		auth.POST("/login", d.Auth.Login)
		auth.POST("/firebase", d.Auth.FirebaseLogin) // Apple / Google
		auth.POST("/refresh", d.Auth.Refresh)
		auth.POST("/logout", d.Auth.Logout)
		auth.POST("/forgot-password", d.Auth.ForgotPassword)
		auth.POST("/reset-password", d.Auth.ResetPassword)
		auth.POST("/verify-email", d.Auth.VerifyEmail)
	}

	// The invite preview is the one read that answers without a session: it is
	// what a link recipient sees before they have an account. It exposes only
	// the title and the members' names — never anything that was said.
	v1.GET("/invites/:id", d.Conversations.Preview)

	// Authenticated auth endpoints.
	authed := v1.Group("/auth", middleware.Auth(d.Tokens))
	{
		authed.GET("/me", d.Auth.Me)
		authed.PATCH("/me", d.Auth.UpdateMe)
		authed.POST("/change-password", d.Auth.ChangePassword)
		authed.DELETE("/account", d.Auth.DeleteAccount)
	}

	// Conversations — the product.
	app := v1.Group("", middleware.Auth(d.Tokens))
	{
		app.GET("/conversations", d.Conversations.List)
		app.POST("/conversations", d.Conversations.Create)
		app.GET("/conversations/:id", d.Conversations.Get)
		app.PATCH("/conversations/:id", d.Conversations.Update)
		app.POST("/conversations/:id/leave", d.Conversations.Leave)

		app.GET("/conversations/:id/messages", d.Conversations.Messages)
		app.POST("/conversations/:id/messages", d.Conversations.Send)
		app.POST("/conversations/:id/ask", d.Conversations.Ask)
		app.POST("/conversations/:id/seen", d.Conversations.MarkSeen)
		app.POST("/conversations/:id/choose", d.Conversations.Choose)

		app.POST("/conversations/:id/members", d.Conversations.AddMembers)
		app.GET("/conversations/:id/invite", d.Conversations.Invite)
		app.POST("/conversations/:id/join", d.Conversations.Join)

		app.POST("/decisions/:id/vote", d.Conversations.Vote)

		// Groups — the standing cast, and what they already know.
		app.GET("/groups", d.Groups.List)
		app.POST("/groups", d.Groups.Create)
		app.GET("/groups/:id", d.Groups.Get)
		app.PATCH("/groups/:id", d.Groups.Update)
		app.DELETE("/groups/:id", d.Groups.Delete)
		app.POST("/groups/:id/members", d.Groups.AddMember)
		app.DELETE("/groups/:id/members/:memberId", d.Groups.RemoveMember)
		app.POST("/groups/:id/memory", d.Groups.AddMemory)
		app.DELETE("/groups/:id/memory/:memoryId", d.Groups.DeleteMemory)
		app.POST("/groups/:id/ask", d.Groups.Ask)

		app.GET("/people/collaborators", d.Groups.Collaborators)

		// Profile — what Chaos knows about you.
		app.GET("/me/profile", d.Profiles.Get)
		app.PATCH("/me/profile", d.Profiles.Update)
		app.POST("/me/facts", d.Profiles.AddFact)
		app.PATCH("/me/facts/:id", d.Profiles.UpdateFact)
		app.DELETE("/me/facts/:id", d.Profiles.DeleteFact)
		app.POST("/me/facts/learn", d.Profiles.Learn)
		app.POST("/me/facts/refresh", d.Profiles.Refresh)
		app.GET("/me/prompts/:source", d.Profiles.Prompt)

		app.GET("/directory/search", d.Conversations.SearchUsers)
		app.POST("/media", d.Media.Upload)
		// Signed in only: this makes the server fetch a URL on request.
		app.GET("/links/preview", d.Links.Preview)
		app.POST("/me/devices", d.Devices.Register)
		app.DELETE("/me/devices/:token", d.Devices.Unregister)
	}

	// User management — admin only.
	users := v1.Group("/users",
		middleware.Auth(d.Tokens),
		middleware.RequireRoles(constants.RoleAdmin),
	)
	{
		users.POST("", d.Users.Create)
		users.GET("", d.Users.List)
		users.GET("/:id", d.Users.Get)
		users.PATCH("/:id", d.Users.Update)
		users.DELETE("/:id", d.Users.Delete)
	}
}
