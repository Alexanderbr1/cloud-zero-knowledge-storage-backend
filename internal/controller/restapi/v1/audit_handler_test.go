package v1_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
)

// ─── mockAuditServiceFull ────────────────────────────────────────────────────
// The base mockAuditService in auth_handler_test.go always returns nil for
// ListEvents. This one lets tests inject errors or canned results.

type mockAuditServiceFull struct {
	events  []entity.AuditEvent
	listErr error
}

func (m *mockAuditServiceFull) LogAsync(_ context.Context, _ entity.AuditEvent) {}
func (m *mockAuditServiceFull) ListEvents(_ context.Context, _ uuid.UUID, _ int, _ time.Time) ([]entity.AuditEvent, error) {
	return m.events, m.listErr
}

// ─── GET /audit ──────────────────────────────────────────────────────────────

func TestListAuditEvents_HappyPath(t *testing.T) {
	events := []entity.AuditEvent{
		{
			ID:        uuid.New(),
			UserID:    uuid.New(),
			EventType: entity.AuditLoginSuccess,
			IPAddress: "127.0.0.1",
			CreatedAt: time.Now(),
		},
	}
	audit := &mockAuditServiceFull{events: events}
	w := doWithAuth(fullRouter(routerOpts{audit: audit}), http.MethodGet, "/audit", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	items, _ := body["events"].([]any)
	if len(items) != 1 {
		t.Errorf("want 1 event, got %d", len(items))
	}
}

func TestListAuditEvents_NoAuth_Returns401(t *testing.T) {
	w := do(fullRouter(routerOpts{}), http.MethodGet, "/audit", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestListAuditEvents_ServiceError_Returns500(t *testing.T) {
	audit := &mockAuditServiceFull{listErr: errors.New("db down")}
	w := doWithAuth(fullRouter(routerOpts{audit: audit}), http.MethodGet, "/audit", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

func TestListAuditEvents_WithLimitParam(t *testing.T) {
	audit := &mockAuditServiceFull{events: []entity.AuditEvent{}}
	w := doWithAuth(fullRouter(routerOpts{audit: audit}), http.MethodGet, "/audit?limit=10", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestListAuditEvents_WithBeforeParam(t *testing.T) {
	before := time.Now().UTC().Format(time.RFC3339Nano)
	audit := &mockAuditServiceFull{events: []entity.AuditEvent{}}
	w := doWithAuth(fullRouter(routerOpts{audit: audit}), http.MethodGet, "/audit?before="+before, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}
