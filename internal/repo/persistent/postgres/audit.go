package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
)

func (s *Storage) InsertAuditEvent(ctx context.Context, e entity.AuditEvent) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO audit_log (user_id, event_type, ip_address, user_agent, resource_id, resource_name)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.UserID, e.EventType, nullableString(e.IPAddress), nullableString(e.UserAgent),
		e.ResourceID, e.ResourceName,
	)
	return err
}

func (s *Storage) ListAuditEvents(ctx context.Context, userID uuid.UUID, limit int, before time.Time) ([]entity.AuditEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, event_type, ip_address::TEXT, user_agent, resource_id, resource_name, created_at
		 FROM audit_log
		 WHERE user_id = $1 AND created_at < $2
		 ORDER BY created_at DESC
		 LIMIT $3`,
		userID, before, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []entity.AuditEvent
	for rows.Next() {
		var e entity.AuditEvent
		var ipAddr, userAgent *string
		if err := rows.Scan(
			&e.ID, &e.UserID, &e.EventType, &ipAddr, &userAgent,
			&e.ResourceID, &e.ResourceName, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		if ipAddr != nil {
			e.IPAddress = *ipAddr
		}
		if userAgent != nil {
			e.UserAgent = *userAgent
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if events == nil {
		return []entity.AuditEvent{}, nil
	}
	return events, nil
}

// nullableString returns nil for empty strings so nullable columns store NULL.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
