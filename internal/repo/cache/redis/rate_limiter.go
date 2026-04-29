package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter is a fixed-window counter backed by Redis.
// Each unique key gets at most Limit increments per Window.
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

// Allow increments the counter for key and reports whether the request is within the limit.
// Returns (true, nil) on Redis error to preserve availability.
func (r *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	fullKey := r.prefix + key
	count, err := r.client.Incr(ctx, fullKey).Result()
	if err != nil {
		return true, err // fail open
	}
	if count == 1 {
		// Set TTL only on the first increment so the window resets naturally.
		r.client.Expire(ctx, fullKey, r.window)
	}
	return count <= r.limit, nil
}
