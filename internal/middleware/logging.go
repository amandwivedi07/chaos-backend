package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/chaosapp/backend/internal/common/constants"
)

// Logging emits one structured line per request (method, path, status,
// latency, request id, client ip). Bodies and tokens are never logged.
func Logging(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("request_id", c.GetString(constants.CtxRequestID)),
			zap.String("ip", c.ClientIP()),
		}
		switch {
		case c.Writer.Status() >= 500:
			log.Error("request", fields...)
		case c.Writer.Status() >= 400:
			log.Warn("request", fields...)
		default:
			log.Info("request", fields...)
		}
	}
}
