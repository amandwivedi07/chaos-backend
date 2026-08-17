package middleware

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chaosapp/backend/internal/common/constants"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/response"
)

// Recovery converts panics into a clean 500 envelope and a stack-traced log —
// a panic must never leak internals to a client or kill the process.
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					zap.Any("panic", r),
					zap.String("path", c.Request.URL.Path),
					zap.String("request_id", c.GetString(constants.CtxRequestID)),
					zap.Stack("stack"),
				)
				response.Error(c, apperrors.Internal(nil))
			}
		}()
		c.Next()
	}
}
