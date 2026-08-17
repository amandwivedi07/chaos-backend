package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/chaosapp/backend/internal/config"
)

// RedisStore implements Store on go-redis.
type RedisStore struct {
	client *redis.Client
}

var _ Store = (*RedisStore)(nil)

func NewRedis(cfg config.RedisConfig) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisStore{client: client}, nil
}

func (s *RedisStore) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := s.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *RedisStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *RedisStore) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return s.client.Del(ctx, keys...).Err()
}

func (s *RedisStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := s.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// Noop is a Store that caches nothing — lets the app run without Redis in dev.
type Noop struct{}

var _ Store = Noop{}

func (Noop) Get(context.Context, string) (string, bool, error)          { return "", false, nil }
func (Noop) Set(context.Context, string, string, time.Duration) error   { return nil }
func (Noop) Delete(context.Context, ...string) error                    { return nil }
func (Noop) Incr(context.Context, string, time.Duration) (int64, error) { return 0, nil }
