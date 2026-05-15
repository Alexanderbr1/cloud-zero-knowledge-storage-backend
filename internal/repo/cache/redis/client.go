package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func NewClient(ctx context.Context, redisURL string) (*redis.Client, func(), error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, func() {}, fmt.Errorf("redis url: %w", err)
	}

	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		return nil, func() {}, fmt.Errorf("redis ping: %w", err)
	}

	return client, func() { _ = client.Close() }, nil
}
