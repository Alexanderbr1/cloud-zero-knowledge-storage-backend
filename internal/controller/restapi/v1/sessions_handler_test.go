package v1_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
	authuc "cloud-backend/internal/usecase/auth"
)

// ─── GET /sessions ───────────────────────────────────────────────────────────

func TestListSessions_HappyPath(t *testing.T) {
	sessionID := uuid.New()
	jwt := &mockParseJWT{userID: uuid.New(), sessionID: sessionID}
	sessions := &mockSessionsService{
		sessions: []entity.DeviceSession{
			{
				ID:           sessionID,
				DeviceName:   "Chrome on macOS",
				IPAddress:    "127.0.0.1",
				UserAgent:    "Mozilla/5.0",
				CreatedAt:    time.Now(),
				LastActiveAt: time.Now(),
			},
		},
	}
	w := doWithAuth(fullRouter(routerOpts{deviceSessions: sessions, jwt: jwt}),
		http.MethodGet, "/sessions", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	items, _ := body["sessions"].([]any)
	if len(items) != 1 {
		t.Fatalf("want 1 session, got %d", len(items))
	}
	// The session whose ID matches the JWT session_id must be marked current.
	sess := items[0].(map[string]any)
	if sess["is_current"] != true {
		t.Error("session matching jwt session_id must have is_current=true")
	}
}

func TestListSessions_NoAuth_Returns401(t *testing.T) {
	w := do(fullRouter(routerOpts{}), http.MethodGet, "/sessions", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestListSessions_ServiceError_Returns500(t *testing.T) {
	sessions := &mockSessionsService{listErr: errors.New("db down")}
	w := doWithAuth(fullRouter(routerOpts{deviceSessions: sessions}), http.MethodGet, "/sessions", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── DELETE /sessions/{id} ───────────────────────────────────────────────────

func TestRevokeSession_HappyPath(t *testing.T) {
	url := "/sessions/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestRevokeSession_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, "/sessions/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestRevokeSession_NotFound_Returns404(t *testing.T) {
	sessions := &mockSessionsService{revokeErr: authuc.ErrSessionNotFound}
	url := "/sessions/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{deviceSessions: sessions}), http.MethodDelete, url, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestRevokeSession_ServiceError_Returns500(t *testing.T) {
	sessions := &mockSessionsService{revokeErr: errors.New("db error")}
	url := "/sessions/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{deviceSessions: sessions}), http.MethodDelete, url, "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── DELETE /sessions (revoke others) ────────────────────────────────────────

func TestRevokeOtherSessions_HappyPath(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, "/sessions", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestRevokeOtherSessions_ServiceError_Returns500(t *testing.T) {
	sessions := &mockSessionsService{revokeOtherErr: errors.New("db error")}
	w := doWithAuth(fullRouter(routerOpts{deviceSessions: sessions}), http.MethodDelete, "/sessions", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}
