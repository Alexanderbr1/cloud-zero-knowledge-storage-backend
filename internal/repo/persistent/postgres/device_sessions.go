package postgres

import (
	"context"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
	authuc "cloud-backend/internal/usecase/auth"
)

var _ authuc.DeviceSessionRepository = (*Storage)(nil)

func (s *Storage) CreateDeviceSession(ctx context.Context, id, userID uuid.UUID, device authuc.DeviceInfo) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO device_sessions (id, user_id, device_name, ip_address, user_agent)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, userID, device.DeviceName, device.IPAddress, device.UserAgent,
	)
	return err
}

func (s *Storage) UpdateLastActive(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE device_sessions SET last_active_at = now() WHERE id = $1`,
		id,
	)
	return err
}

func (s *Storage) ListActiveSessions(ctx context.Context, userID uuid.UUID) ([]entity.DeviceSession, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, device_name, ip_address, user_agent, created_at, last_active_at
		 FROM device_sessions
		 WHERE user_id = $1 AND revoked_at IS NULL
		 ORDER BY last_active_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []entity.DeviceSession
	for rows.Next() {
		var ds entity.DeviceSession
		if err := rows.Scan(
			&ds.ID, &ds.UserID, &ds.DeviceName, &ds.IPAddress, &ds.UserAgent,
			&ds.CreatedAt, &ds.LastActiveAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, ds)
	}
	return sessions, rows.Err()
}

func (s *Storage) RevokeSession(ctx context.Context, id, userID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE device_sessions
		 SET revoked_at = now()
		 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
		id, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return authuc.ErrSessionNotFound
	}
	// Revoke all refresh tokens for this device so they cannot be used after session revocation.
	_, err = s.pool.Exec(ctx,
		`UPDATE refresh_sessions SET revoked_at = now()
		 WHERE device_session_id = $1 AND revoked_at IS NULL`,
		id,
	)
	return err
}

func (s *Storage) RevokeOtherSessions(ctx context.Context, userID, exceptID uuid.UUID) ([]uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx,
		`UPDATE device_sessions
		 SET revoked_at = now()
		 WHERE user_id = $1 AND id != $2 AND revoked_at IS NULL
		 RETURNING id`,
		userID, exceptID,
	)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE refresh_sessions SET revoked_at = now()
		 WHERE user_id = $1 AND device_session_id != $2 AND revoked_at IS NULL`,
		userID, exceptID,
	); err != nil {
		return nil, err
	}

	return ids, tx.Commit(ctx)
}

// RevokeOrphanedSessions revokes device sessions that have no active refresh tokens.
// Pass uuid.Nil to revoke across all users (used by the background cleanup job).
func (s *Storage) RevokeOrphanedSessions(ctx context.Context, userID uuid.UUID) error {
	var err error
	if userID == uuid.Nil {
		_, err = s.pool.Exec(ctx,
			`UPDATE device_sessions
			 SET revoked_at = now()
			 WHERE revoked_at IS NULL
			   AND NOT EXISTS (
			     SELECT 1 FROM refresh_sessions rs
			     WHERE rs.device_session_id = device_sessions.id
			       AND rs.revoked_at IS NULL
			       AND rs.expires_at > now()
			   )`,
		)
	} else {
		_, err = s.pool.Exec(ctx,
			`UPDATE device_sessions
			 SET revoked_at = now()
			 WHERE user_id = $1
			   AND revoked_at IS NULL
			   AND NOT EXISTS (
			     SELECT 1 FROM refresh_sessions rs
			     WHERE rs.device_session_id = device_sessions.id
			       AND rs.revoked_at IS NULL
			       AND rs.expires_at > now()
			   )`,
			userID,
		)
	}
	return err
}
