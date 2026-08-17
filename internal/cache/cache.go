// Package cache defines the caching port and its Redis adapter.
// Services depend on Store, never on Redis — swap for memcached/in-memory
// without touching business logic.
package cache

import (
	"context"
	"time"
)

// Store is the caching port used by services (cache-aside pattern).
type Store interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	// Incr increments a counter and sets ttl on first increment (rate limiting).
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}
