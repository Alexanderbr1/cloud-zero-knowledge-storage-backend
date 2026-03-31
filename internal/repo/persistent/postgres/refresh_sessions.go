package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Storage) CreateRefreshSession(ctx context.Context, sessionID, userID uuid.UUID, refreshTokenHash []byte, expiresAt time.Time) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO refresh_sessions (id, user_id, refresh_token_hash, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		sessionID, userID, refreshTokenHash, expiresAt,
	)
	return err
}

// ConsumeRefreshSession atomically revokes a valid refresh session (single-use) and returns its userID.
func (s *Storage) ConsumeRefreshSession(ctx context.Context, refreshTokenHash []byte) (sessionID, userID uuid.UUID, ok bool, err error) {
	err = s.Pool.QueryRow(ctx,
		`UPDATE refresh_sessions
		   SET revoked_at = now()
		 WHERE refresh_token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > now()
		 RETURNING id, user_id`,
		refreshTokenHash,
	).Scan(&sessionID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.UUID{}, uuid.UUID{}, false, nil
	}
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, false, err
	}
	return sessionID, userID, true, nil
}

// RevokeRefreshSessionByHash revokes a session if it exists (idempotent).
func (s *Storage) RevokeRefreshSessionByHash(ctx context.Context, refreshTokenHash []byte) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE refresh_sessions
		   SET revoked_at = now()
		 WHERE refresh_token_hash = $1
		   AND revoked_at IS NULL`,
		refreshTokenHash,
	)
	return err
}
