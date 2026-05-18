package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// incrExpireScript atomically increments a counter and sets TTL on first call.
// KEYS[1] = key, ARGV[1] = window seconds. Returns the new count.
var incrExpireScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
`)

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
	windowSec := int64(r.window.Seconds())
	count, err := incrExpireScript.Run(ctx, r.client, []string{fullKey}, windowSec).Int64()
	if err != nil {
		return false, err
	}
	return count <= r.limit, nil
}
