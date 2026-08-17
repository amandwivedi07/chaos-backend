package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/chaosapp/backend/internal/common/constants"
)

// RequestID assigns (or propagates) a request id and echoes it in the response
// header so client logs and server logs can be correlated.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(constants.HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(constants.CtxRequestID, id)
		c.Header(constants.HeaderRequestID, id)
		c.Next()
	}
}
