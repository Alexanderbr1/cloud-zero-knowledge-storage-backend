package restapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/internal/controller/restapi"
)

// ─── Mocks ────────────────────────────────────────────────────────────────────

type mockJWT struct {
	userID    uuid.UUID
	sessionID uuid.UUID
	err       error
}

func (m *mockJWT) ParseAccessToken(_ string) (uuid.UUID, uuid.UUID, error) {
	return m.userID, m.sessionID, m.err
}

type mockBlocklist struct {
	blocked bool
	err     error
}

func (m *mockBlocklist) IsBlocked(_ context.Context, _ uuid.UUID) (bool, error) {
	return m.blocked, m.err
}

type mockRateLimiter struct {
	allowed bool
	err     error
}

func (m *mockRateLimiter) Allow(_ context.Context, _ string) (bool, error) {
	return m.allowed, m.err
}

var nopLog = zerolog.Nop()

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// ─── AuthMiddleware ───────────────────────────────────────────────────────────

func TestAuthMiddleware_NoHeader(t *testing.T) {
	mw := restapi.AuthMiddleware(&mockJWT{}, &mockBlocklist{}, nopLog)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidScheme(t *testing.T) {
	mw := restapi.AuthMiddleware(&mockJWT{}, &mockBlocklist{}, nopLog)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_EmptyToken(t *testing.T) {
	mw := restapi.AuthMiddleware(&mockJWT{}, &mockBlocklist{}, nopLog)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer   ")
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	mw := restapi.AuthMiddleware(
		&mockJWT{err: errors.New("bad token")},
		&mockBlocklist{},
		nopLog,
	)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bad.token")
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_BlockedSession(t *testing.T) {
	mw := restapi.AuthMiddleware(
		&mockJWT{userID: uuid.New(), sessionID: uuid.New()},
		&mockBlocklist{blocked: true},
		nopLog,
	)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid.token")
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuthMiddleware_BlocklistError(t *testing.T) {
	mw := restapi.AuthMiddleware(
		&mockJWT{userID: uuid.New(), sessionID: uuid.New()},
		&mockBlocklist{err: errors.New("redis down")},
		nopLog,
	)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid.token")
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", w.Code)
	}
}

func TestAuthMiddleware_ValidToken_PopulatesContext(t *testing.T) {
	wantUID := uuid.New()
	wantSID := uuid.New()
	var gotUID uuid.UUID
	var gotSID uuid.UUID

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := restapi.UserIDFromContext(r.Context())
		if !ok {
			t.Error("userID not found in context")
		}
		gotUID = id
		gotSID = restapi.SessionIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	mw := restapi.AuthMiddleware(
		&mockJWT{userID: wantUID, sessionID: wantSID},
		&mockBlocklist{blocked: false},
		nopLog,
	)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer valid.token")
	w := httptest.NewRecorder()
	mw(handler).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if gotUID != wantUID {
		t.Errorf("context userID: want %v, got %v", wantUID, gotUID)
	}
	if gotSID != wantSID {
		t.Errorf("context sessionID: want %v, got %v", wantSID, gotSID)
	}
}

// ─── RateLimitMiddleware ──────────────────────────────────────────────────────

func TestRateLimit_Allowed(t *testing.T) {
	mw := restapi.RateLimitMiddleware(&mockRateLimiter{allowed: true}, func(*http.Request) string { return "k" })
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}

func TestRateLimit_Denied(t *testing.T) {
	mw := restapi.RateLimitMiddleware(&mockRateLimiter{allowed: false}, func(*http.Request) string { return "k" })
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("want 429, got %d", w.Code)
	}
}

func TestRateLimit_Error_Returns503(t *testing.T) {
	mw := restapi.RateLimitMiddleware(
		&mockRateLimiter{allowed: false, err: errors.New("redis down")},
		func(*http.Request) string { return "k" },
	)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503 when rate limiter errors, got %d", w.Code)
	}
}
