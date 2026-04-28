package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authuc "cloud-backend/internal/usecase/auth"
)

var _ authuc.SessionRepository = (*Storage)(nil)

func (s *Storage) CreateRefreshSession(ctx context.Context, p authuc.RefreshSessionParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO refresh_sessions (id, user_id, device_session_id, refresh_token_hash, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		p.SessionID, p.UserID, p.DeviceSessionID, p.TokenHash, p.ExpiresAt,
	)
	return err
}

// ConsumeRefreshSession атомарно отзывает валидную сессию (single-use) и возвращает userID + deviceSessionID.
// Также проверяет, что device_session не отозвана — иначе токен считается невалидным.
func (s *Storage) ConsumeRefreshSession(ctx context.Context, tokenHash []byte) (authuc.ConsumedSession, bool, error) {
	var cs authuc.ConsumedSession
	err := s.pool.QueryRow(ctx,
		`UPDATE refresh_sessions
		    SET revoked_at = now()
		   FROM device_sessions
		  WHERE refresh_sessions.refresh_token_hash = $1
		    AND refresh_sessions.revoked_at IS NULL
		    AND refresh_sessions.expires_at > now()
		    AND device_sessions.id = refresh_sessions.device_session_id
		    AND device_sessions.revoked_at IS NULL
		RETURNING refresh_sessions.id, refresh_sessions.user_id, refresh_sessions.device_session_id`,
		tokenHash,
	).Scan(&cs.SessionID, &cs.UserID, &cs.DeviceSessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return authuc.ConsumedSession{}, false, nil
	}
	if err != nil {
		return authuc.ConsumedSession{}, false, err
	}
	return cs, true, nil
}

// ConsumeAndGetSession атомарно отзывает refresh-токен и возвращает userID + deviceSessionID.
// Используется при логауте.
func (s *Storage) ConsumeAndGetSession(ctx context.Context, refreshTokenHash []byte) (userID, deviceSessionID uuid.UUID, err error) {
	err = s.pool.QueryRow(ctx,
		`UPDATE refresh_sessions
		   SET revoked_at = now()
		 WHERE refresh_token_hash = $1
		   AND revoked_at IS NULL
		 RETURNING user_id, device_session_id`,
		refreshTokenHash,
	).Scan(&userID, &deviceSessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, nil
	}
	return userID, deviceSessionID, err
}
