// Package middleware contains all Gin middleware. Middleware never holds
// business logic — it authenticates, decorates context, logs, and protects.
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/chaosapp/backend/internal/auth/jwt"
	"github.com/chaosapp/backend/internal/common/constants"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/response"
)

// Auth validates the Bearer access token and stores user id + role in context.
func Auth(tokens jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			response.Error(c, apperrors.Unauthorized("Missing or malformed Authorization header"))
			return
		}
		claims, err := tokens.ParseAccess(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			response.Error(c, apperrors.Unauthorized("Invalid or expired token"))
			return
		}
		c.Set(constants.CtxUserID, claims.UserID)
		c.Set(constants.CtxUserRole, claims.Role)
		c.Next()
	}
}

// RequireRoles allows only the listed roles (use after Auth).
func RequireRoles(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role := c.GetString(constants.CtxUserRole)
		if _, ok := allowed[role]; !ok {
			response.Error(c, apperrors.Forbidden("You do not have access to this resource"))
			return
		}
		c.Next()
	}
}
