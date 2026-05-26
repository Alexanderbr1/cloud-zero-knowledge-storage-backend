package audit_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/internal/entity"
	"cloud-backend/internal/usecase/audit"
)

// ─── mock repo ────────────────────────────────────────────────────────────────

type mockAuditRepo struct {
	mu          sync.Mutex
	insertCalls int64
	insertErr   error

	listResult []entity.AuditEvent
	listErr    error

	// capturedLimits records every limit argument passed to ListAuditEvents.
	capturedLimits []int
	// capturedBefores records every before argument passed to ListAuditEvents.
	capturedBefores []time.Time
}

func (m *mockAuditRepo) InsertAuditEvent(_ context.Context, _ entity.AuditEvent) error {
	atomic.AddInt64(&m.insertCalls, 1)
	m.mu.Lock()
	err := m.insertErr
	m.mu.Unlock()
	return err
}

func (m *mockAuditRepo) ListAuditEvents(_ context.Context, _ uuid.UUID, limit int, before time.Time) ([]entity.AuditEvent, error) {
	m.mu.Lock()
	m.capturedLimits = append(m.capturedLimits, limit)
	m.capturedBefores = append(m.capturedBefores, before)
	m.mu.Unlock()
	return m.listResult, m.listErr
}

func newSvc(repo *mockAuditRepo) *audit.Service {
	return &audit.Service{Repo: repo, Logger: zerolog.Nop()}
}

// ─── ListEvents — limit clamping ──────────────────────────────────────────────

func TestListEvents_LimitClamping(t *testing.T) {
	tests := []struct {
		name      string
		input     int
		wantLimit int
	}{
		{"zero clamps to 50", 0, 50},
		{"negative clamps to 50", -1, 50},
		{"over 100 clamps to 50", 101, 50},
		{"exactly 100 passes", 100, 100},
		{"in range passes", 25, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockAuditRepo{}
			svc := newSvc(repo)

			_, err := svc.ListEvents(context.Background(), uuid.New(), tt.input, time.Now())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := repo.capturedLimits[0]; got != tt.wantLimit {
				t.Errorf("limit: got %d, want %d", got, tt.wantLimit)
			}
		})
	}
}

// ─── ListEvents — zero before → uses time.Now() ───────────────────────────────

func TestListEvents_ZeroBefore_UsesNow(t *testing.T) {
	repo := &mockAuditRepo{}
	svc := newSvc(repo)

	before := time.Now()
	_, _ = svc.ListEvents(context.Background(), uuid.New(), 10, time.Time{})
	after := time.Now()

	captured := repo.capturedBefores[0]
	if captured.Before(before) || captured.After(after) {
		t.Errorf("expected before to be ~now, got %v", captured)
	}
}

// ─── ListEvents — repo error propagation ─────────────────────────────────────

func TestListEvents_RepoError_Propagates(t *testing.T) {
	repo := &mockAuditRepo{listErr: errors.New("db down")}
	svc := newSvc(repo)

	_, err := svc.ListEvents(context.Background(), uuid.New(), 10, time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── ListEvents — results pass through ───────────────────────────────────────

func TestListEvents_ReturnsRepoResult(t *testing.T) {
	events := []entity.AuditEvent{
		{EventType: entity.AuditLoginSuccess},
		{EventType: entity.AuditFileUploaded},
	}
	repo := &mockAuditRepo{listResult: events}
	svc := newSvc(repo)

	got, err := svc.ListEvents(context.Background(), uuid.New(), 10, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("events: got %d, want 2", len(got))
	}
}

// ─── LogAsync — success on first attempt ─────────────────────────────────────

func TestLogAsync_SuccessOnFirstAttempt_InsertCalledOnce(t *testing.T) {
	repo := &mockAuditRepo{}
	svc := newSvc(repo)

	svc.LogAsync(context.Background(), entity.AuditEvent{EventType: entity.AuditLoginSuccess})

	// Give the goroutine time to finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&repo.insertCalls) >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := atomic.LoadInt64(&repo.insertCalls); got != 1 {
		t.Errorf("insert calls: got %d, want 1", got)
	}
}

// ─── LogAsync — retry on error ────────────────────────────────────────────────

func TestLogAsync_RetriesThreeTimes_ThenGivesUp(t *testing.T) {
	repo := &mockAuditRepo{insertErr: errors.New("db unavailable")}
	svc := newSvc(repo)

	svc.LogAsync(context.Background(), entity.AuditEvent{EventType: entity.AuditLoginSuccess})

	// Retry schedule: immediate, +500ms, +1000ms = ~1.5s total.
	// We wait up to 4 seconds to account for test overhead.
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&repo.insertCalls) >= 3 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if got := atomic.LoadInt64(&repo.insertCalls); got != 3 {
		t.Errorf("insert calls: got %d, want 3", got)
	}
}

func TestLogAsync_SucceedsOnSecondAttempt_StopsRetrying(t *testing.T) {
	var calls atomic.Int64
	// Fail the first insert, succeed on subsequent ones.
	repo := &mockAuditRepo{}
	repo.mu.Lock()
	repo.insertErr = errors.New("transient")
	repo.mu.Unlock()

	// We override insertErr after first call using a custom mock.
	custom := &countingRepo{failFirst: true}
	svcCustom := &audit.Service{Repo: custom, Logger: zerolog.Nop()}

	svcCustom.LogAsync(context.Background(), entity.AuditEvent{EventType: entity.AuditFileUploaded})
	_ = calls // prevent unused warning

	// Wait for goroutine to complete (success on 2nd attempt: ~500ms).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if custom.calls.Load() >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if got := custom.calls.Load(); got != 2 {
		t.Errorf("insert calls: got %d, want 2 (fail first, succeed second)", got)
	}
}

// countingRepo fails the first call and succeeds thereafter.
type countingRepo struct {
	failFirst bool
	calls     atomic.Int64

	listResult []entity.AuditEvent
}

func (r *countingRepo) InsertAuditEvent(_ context.Context, _ entity.AuditEvent) error {
	n := r.calls.Add(1)
	if r.failFirst && n == 1 {
		return errors.New("transient error")
	}
	return nil
}

func (r *countingRepo) ListAuditEvents(_ context.Context, _ uuid.UUID, _ int, _ time.Time) ([]entity.AuditEvent, error) {
	return r.listResult, nil
}
