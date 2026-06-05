package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	authuc "cloud-backend/internal/usecase/auth"
)

func (s *Storage) CreateRefreshSession(ctx context.Context, p authuc.RefreshSessionParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO refresh_sessions (id, user_id, device_session_id, refresh_token_hash, client_key, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		p.SessionID, p.UserID, p.DeviceSessionID, p.TokenHash, p.ClientKey, p.ExpiresAt,
	)
	return err
}

// FindRevokedSession returns a session that was already consumed (revoked_at IS NOT NULL).
// A non-expired token that appears consumed indicates reuse — the token was rotated and
// someone is presenting the old value again.
func (s *Storage) FindRevokedSession(ctx context.Context, tokenHash []byte) (authuc.ConsumedSession, bool, error) {
	var cs authuc.ConsumedSession
	err := s.pool.QueryRow(ctx,
		`SELECT user_id, device_session_id
		 FROM refresh_sessions
		 WHERE refresh_token_hash = $1
		   AND revoked_at IS NOT NULL`,
		tokenHash,
	).Scan(&cs.UserID, &cs.DeviceSessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return authuc.ConsumedSession{}, false, nil
	}
	if err != nil {
		return authuc.ConsumedSession{}, false, err
	}
	return cs, true, nil
}

// ConsumeRefreshSession rejects tokens whose device session was separately revoked.
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
		 RETURNING refresh_sessions.id, refresh_sessions.user_id, refresh_sessions.device_session_id, refresh_sessions.client_key`,
		tokenHash,
	).Scan(&cs.SessionID, &cs.UserID, &cs.DeviceSessionID, &cs.ClientKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return authuc.ConsumedSession{}, false, nil
	}
	if err != nil {
		return authuc.ConsumedSession{}, false, err
	}
	return cs, true, nil
}
