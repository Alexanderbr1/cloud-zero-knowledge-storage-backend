package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// On Redis failure the request is allowed through (fail open).
type RateLimiter struct {
	client *redis.Client
	prefix string
	limit  int64
	window time.Duration
}

func NewRateLimiter(client *redis.Client, prefix string, limit int64, window time.Duration) *RateLimiter {
	return &RateLimiter{client: client, prefix: prefix, limit: limit, window: window}
}

func (r *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	fullKey := r.prefix + key
	count, err := r.client.Incr(ctx, fullKey).Result()
	if err != nil {
		return true, err // fail open
	}
	if count == 1 {
		// Set TTL only on first increment so the window resets naturally.
		// Treat a failed Expire as a transient error and fail open rather than
		// blocking the key forever.
		if err := r.client.Expire(ctx, fullKey, r.window).Err(); err != nil {
			return true, err
		}
	}
	return count <= r.limit, nil
}
