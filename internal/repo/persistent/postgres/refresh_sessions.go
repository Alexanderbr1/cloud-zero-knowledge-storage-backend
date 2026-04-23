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

func (s *Storage) CreateRefreshSession(
	ctx context.Context,
	sessionID, userID, deviceSessionID uuid.UUID,
	refreshTokenHash []byte,
	expiresAt time.Time,
) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO refresh_sessions (id, user_id, device_session_id, refresh_token_hash, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		sessionID, userID, deviceSessionID, refreshTokenHash, expiresAt,
	)
	return err
}

// ConsumeRefreshSession атомарно отзывает валидную сессию (single-use) и возвращает userID + deviceSessionID.
// Также проверяет, что device_session не отозвана — иначе токен считается невалидным.
func (s *Storage) ConsumeRefreshSession(ctx context.Context, refreshTokenHash []byte) (
	sessionID, userID, deviceSessionID uuid.UUID, ok bool, err error,
) {
	err = s.pool.QueryRow(ctx,
		`UPDATE refresh_sessions
		    SET revoked_at = now()
		   FROM device_sessions
		  WHERE refresh_sessions.refresh_token_hash = $1
		    AND refresh_sessions.revoked_at IS NULL
		    AND refresh_sessions.expires_at > now()
		    AND device_sessions.id = refresh_sessions.device_session_id
		    AND device_sessions.revoked_at IS NULL
		RETURNING refresh_sessions.id, refresh_sessions.user_id, refresh_sessions.device_session_id`,
		refreshTokenHash,
	).Scan(&sessionID, &userID, &deviceSessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, false, err
	}
	return sessionID, userID, deviceSessionID, true, nil
}

// RevokeRefreshSessionByHash отзывает refresh-токен (идемпотентно).
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

// ConsumeAndGetDeviceSession атомарно отзывает refresh-токен и возвращает device_session_id.
// Используется при логауте, чтобы в одном запросе и отозвать токен, и знать какое устройство удалять.
func (s *Storage) ConsumeAndGetDeviceSession(ctx context.Context, refreshTokenHash []byte) (uuid.UUID, error) {
	var deviceSessionID uuid.UUID
	err := s.pool.QueryRow(ctx,
		`UPDATE refresh_sessions
		   SET revoked_at = now()
		 WHERE refresh_token_hash = $1
		   AND revoked_at IS NULL
		 RETURNING device_session_id`,
		refreshTokenHash,
	).Scan(&deviceSessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	return deviceSessionID, err
}

// RevokeAllByDeviceSession отзывает все токены устройства (не нужен при ON DELETE CASCADE, но полезен явно).
func (s *Storage) RevokeAllByDeviceSession(ctx context.Context, deviceSessionID uuid.UUID) error {
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`UPDATE refresh_sessions SET revoked_at = $1
		 WHERE device_session_id = $2 AND revoked_at IS NULL`,
		now, deviceSessionID,
	)
	return err
}
