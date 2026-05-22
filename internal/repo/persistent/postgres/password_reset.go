package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var _ interface {
	CreateResetToken(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error
	ConsumeResetToken(ctx context.Context, tokenHash []byte) (uuid.UUID, bool, error)
	GetUserIDFromResetToken(ctx context.Context, tokenHash []byte) (uuid.UUID, bool, error)
	InvalidateUserTokens(ctx context.Context, userID uuid.UUID) error
} = (*Storage)(nil)

func (s *Storage) CreateResetToken(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		 VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

// GetUserIDFromResetToken peeks at a valid token without consuming it.
// Returns (uuid.Nil, false, nil) if not found, expired, or already used.
func (s *Storage) GetUserIDFromResetToken(ctx context.Context, tokenHash []byte) (uuid.UUID, bool, error) {
	var userID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT user_id FROM password_reset_tokens
		 WHERE token_hash = $1
		   AND used_at IS NULL
		   AND expires_at > now()`,
		tokenHash,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return userID, true, nil
}

// InvalidateUserTokens marks all active reset tokens for a user as used.
func (s *Storage) InvalidateUserTokens(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE password_reset_tokens
		 SET used_at = now()
		 WHERE user_id = $1 AND used_at IS NULL`,
		userID,
	)
	return err
}

// ConsumeResetToken marks the token as used and returns the owner's userID.
// Returns (uuid.Nil, false, nil) if the token is not found, expired, or already used.
func (s *Storage) ConsumeResetToken(ctx context.Context, tokenHash []byte) (uuid.UUID, bool, error) {
	var userID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`UPDATE password_reset_tokens
		 SET used_at = now()
		 WHERE token_hash = $1
		   AND used_at IS NULL
		   AND expires_at > now()
		 RETURNING user_id`,
		tokenHash,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return userID, true, nil
}
