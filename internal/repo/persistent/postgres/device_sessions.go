package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cloud-backend/internal/entity"
	authuc "cloud-backend/internal/usecase/auth"
)

var _ authuc.DeviceSessionRepository = (*Storage)(nil)

func (s *Storage) CreateDeviceSession(
	ctx context.Context,
	id, userID uuid.UUID,
	deviceName, ipAddress, userAgent string,
) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO device_sessions (id, user_id, device_name, ip_address, user_agent)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, userID, deviceName, ipAddress, userAgent,
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

func (s *Storage) RevokeOtherSessions(ctx context.Context, userID, exceptID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE device_sessions
		 SET revoked_at = now()
		 WHERE user_id = $1 AND id != $2 AND revoked_at IS NULL`,
		userID, exceptID,
	)
	if err != nil {
		return err
	}
	// Revoke refresh tokens for all devices that were just revoked.
	_, err = s.pool.Exec(ctx,
		`UPDATE refresh_sessions SET revoked_at = now()
		 WHERE user_id = $1 AND device_session_id != $2 AND revoked_at IS NULL`,
		userID, exceptID,
	)
	return err
}

// GetDeviceSessionByRefreshHash возвращает device_session_id для отзыва при логауте.
func (s *Storage) GetDeviceSessionByRefreshHash(ctx context.Context, hash []byte) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT device_session_id FROM refresh_sessions
		 WHERE refresh_token_hash = $1 AND revoked_at IS NULL`,
		hash,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	return id, err
}

// RevokeDeviceSessionAt используется при логауте: отзывает устройство по его ID.
func (s *Storage) RevokeDeviceSessionByID(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return nil
	}
	now := time.Now()
	_, err := s.pool.Exec(ctx,
		`UPDATE device_sessions SET revoked_at = $1 WHERE id = $2 AND revoked_at IS NULL`,
		now, id,
	)
	return err
}
