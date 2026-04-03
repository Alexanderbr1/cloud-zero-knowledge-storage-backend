package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authuc "cloud-backend/internal/usecase/auth"
)

var _ authuc.SessionRepository = (*Storage)(nil)

func (s *Storage) CreateRefreshSession(ctx context.Context, sessionID, userID uuid.UUID, refreshTokenHash []byte, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO refresh_sessions (id, user_id, refresh_token_hash, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		sessionID, userID, refreshTokenHash, expiresAt,
	)
	return err
}

// ConsumeRefreshSession атомарно отзывает валидную сессию (single-use) и возвращает userID.
func (s *Storage) ConsumeRefreshSession(ctx context.Context, refreshTokenHash []byte) (sessionID, userID uuid.UUID, ok bool, err error) {
	err = s.pool.QueryRow(ctx,
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

// RevokeRefreshSessionByHash отзывает сессию, если она существует (идемпотентно).
func (s *Storage) RevokeRefreshSessionByHash(ctx context.Context, refreshTokenHash []byte) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE refresh_sessions
		   SET revoked_at = now()
		 WHERE refresh_token_hash = $1
		   AND revoked_at IS NULL`,
		refreshTokenHash,
	)
	return err
}
