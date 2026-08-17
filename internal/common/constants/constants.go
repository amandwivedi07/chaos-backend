// Package constants centralizes cross-cutting literal values.
package constants

// Context keys set by middleware, read by handlers.
const (
	CtxUserID    = "ctx_user_id"
	CtxUserRole  = "ctx_user_role"
	CtxRequestID = "ctx_request_id"
)

// Roles.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// Token types embedded in JWT claims.
const (
	TokenAccess  = "access"
	TokenRefresh = "refresh"
)

// HeaderRequestID is echoed back on every response.
const HeaderRequestID = "X-Request-ID"

// Palettes are the five member colours. Every avatar, name label and vote chip
// in the app is tinted from this list, and the client maps each id to the
// matching --member-N token — so the set must stay exactly five, in this
// order.
var Palettes = []string{"tide", "mint", "rose", "sun", "iris"}
