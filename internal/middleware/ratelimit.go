package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/chaosapp/backend/internal/cache"
	"github.com/chaosapp/backend/internal/common/constants"
	apperrors "github.com/chaosapp/backend/internal/common/errors"
	"github.com/chaosapp/backend/internal/common/response"
)

// RateLimit is a fixed-window limiter backed by the cache.Store port —
// with Redis it is naturally distributed across instances; with the Noop
// store it is disabled (dev). Keyed by user id when authenticated, else IP.
func RateLimit(store cache.Store, requestsPerMinute int) gin.HandlerFunc {
	window := time.Minute
	return func(c *gin.Context) {
		key := c.ClientIP()
		if v, ok := c.Get(constants.CtxUserID); ok {
			key = fmt.Sprint(v)
		}
		count, err := store.Incr(c.Request.Context(), "rl:"+key, window)
		if err == nil && count > int64(requestsPerMinute) {
			response.Error(c, apperrors.RateLimited("Too many requests — slow down"))
			return
		}
		c.Next()
	}
}
