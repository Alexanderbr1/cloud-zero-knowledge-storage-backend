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

type AuditLogger interface {
	LogAsync(ctx context.Context, e entity.AuditEvent)
}

const queueSize = 512

type Service struct {
	Repo   AuditRepo
	Logger zerolog.Logger
	queue  chan entity.AuditEvent
}

func NewService(repo AuditRepo, log zerolog.Logger) *Service {
	return &Service{
		Repo:   repo,
		Logger: log,
		queue:  make(chan entity.AuditEvent, queueSize),
	}
}

// Start launches the background worker. Must be called once after construction.
// The worker stops when ctx is cancelled and drains any queued events before returning.
func (s *Service) Start(ctx context.Context) {
	go s.worker(ctx)
}

func (s *Service) worker(ctx context.Context) {
	for {
		select {
		case e := <-s.queue:
			s.insertWithRetry(e)
		case <-ctx.Done():
			// Drain remaining events before exiting.
			for {
				select {
				case e := <-s.queue:
					s.insertWithRetry(e)
				default:
					return
				}
			}
		}
	}
}

// LogAsync enqueues an audit event for asynchronous insertion.
// Drops the event and logs a warning if the queue is full.
func (s *Service) LogAsync(_ context.Context, e entity.AuditEvent) {
	select {
	case s.queue <- e:
	default:
		s.Logger.Warn().Str("event_type", e.EventType).Msg("audit queue full, event dropped")
	}
}

func (s *Service) insertWithRetry(e entity.AuditEvent) {
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
