package redis

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const keyPrefix = "blocked_session:"

// SessionBlocklist stores revoked device session IDs in Redis with TTL equal to
// the access token lifetime. Once the token would expire naturally the Redis
// entry is also gone, keeping the blocklist small.
type SessionBlocklist struct {
	client *redis.Client
}

func NewSessionBlocklist(client *redis.Client) *SessionBlocklist {
	return &SessionBlocklist{client: client}
}

// Block marks a single session as revoked for the given TTL.
func (b *SessionBlocklist) Block(ctx context.Context, id uuid.UUID, ttl time.Duration) error {
	return b.client.Set(ctx, keyPrefix+id.String(), 1, ttl).Err()
}

// BlockBatch marks multiple sessions as revoked in a single pipeline round-trip.
func (b *SessionBlocklist) BlockBatch(ctx context.Context, ids []uuid.UUID, ttl time.Duration) error {
	if len(ids) == 0 {
		return nil
	}
	pipe := b.client.Pipeline()
	for _, id := range ids {
		pipe.Set(ctx, keyPrefix+id.String(), 1, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// IsBlocked reports whether the session has been explicitly revoked.
func (b *SessionBlocklist) IsBlocked(ctx context.Context, id uuid.UUID) (bool, error) {
	n, err := b.client.Exists(ctx, keyPrefix+id.String()).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
