package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/chaosapp/backend/internal/common/constants"
)

// UserID extracts the authenticated user's id set by Auth.
// Handlers use this instead of touching context keys directly.
func UserID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(constants.CtxUserID)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}
