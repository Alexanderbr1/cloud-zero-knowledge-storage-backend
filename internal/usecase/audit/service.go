package audit

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/internal/entity"
)

type AuditRepo interface {
	InsertAuditEvent(ctx context.Context, e entity.AuditEvent) error
	ListAuditEvents(ctx context.Context, userID uuid.UUID, limit int, before time.Time) ([]entity.AuditEvent, error)
}

// AuditLogger is the interface auth/storage usecases depend on.
type AuditLogger interface {
	LogAsync(ctx context.Context, e entity.AuditEvent)
}

type Service struct {
	Repo   AuditRepo
	Logger zerolog.Logger
}

func (s *Service) LogAsync(_ context.Context, e entity.AuditEvent) {
	go func() {
		delays := [3]time.Duration{0, 500 * time.Millisecond, time.Second}
		var err error
		for _, d := range delays {
			if d > 0 {
				time.Sleep(d)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = s.Repo.InsertAuditEvent(ctx, e)
			cancel()
			if err == nil {
				return
			}
		}
		s.Logger.Warn().Err(err).Str("event_type", e.EventType).Msg("audit log insert failed after 3 retries")
	}()
}

func (s *Service) ListEvents(ctx context.Context, userID uuid.UUID, limit int, before time.Time) ([]entity.AuditEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if before.IsZero() {
		before = time.Now()
	}
	return s.Repo.ListAuditEvents(ctx, userID, limit, before)
}
