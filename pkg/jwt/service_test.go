package jwt

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestService(ttl time.Duration) *Service {
	return NewService([]byte("test-secret-32-bytes-1234567890!"), ttl)
}

// TestJWTRoundTrip issues a token and verifies that ParseAccessToken returns
// the same userID and deviceSessionID.
func TestJWTRoundTrip(t *testing.T) {
	svc := newTestService(15 * time.Minute)
	userID := uuid.New()
	sessionID := uuid.New()

	token, exp, err := svc.IssueAccess(userID, sessionID)
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}
	if exp != 900 {
		t.Errorf("expiresIn: want 900, got %d", exp)
	}

	gotUser, gotSession, err := svc.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if gotUser != userID {
		t.Errorf("userID: want %v, got %v", userID, gotUser)
	}
	if gotSession != sessionID {
		t.Errorf("sessionID: want %v, got %v", sessionID, gotSession)
	}
}

// TestJWTExpiredToken verifies that a token issued with a negative TTL is rejected.
func TestJWTExpiredToken(t *testing.T) {
	svc := newTestService(-time.Minute)
	token, _, err := svc.IssueAccess(uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("IssueAccess: %v", err)
	}
	_, _, err = svc.ParseAccessToken(token)
	if err != ErrInvalidToken {
		t.Fatalf("want ErrInvalidToken for expired token, got %v", err)
	}
}

// TestJWTWrongSecret verifies that a token signed with a different secret is rejected.
func TestJWTWrongSecret(t *testing.T) {
	svc := newTestService(time.Minute)
	token, _, err := svc.IssueAccess(uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}

	other := NewService([]byte("other-secret-32-bytes-1234567890"), time.Minute)
	_, _, err = other.ParseAccessToken(token)
	if err != ErrInvalidToken {
		t.Fatalf("want ErrInvalidToken for wrong secret, got %v", err)
	}
}

// TestJWTNilUserID verifies that IssueAccess rejects a nil UUID.
func TestJWTNilUserID(t *testing.T) {
	svc := newTestService(time.Minute)
	_, _, err := svc.IssueAccess(uuid.Nil, uuid.New())
	if err == nil {
		t.Fatal("expected error for nil userID")
	}
}

// TestJWTMalformedToken verifies that a garbage string is rejected.
func TestJWTMalformedToken(t *testing.T) {
	svc := newTestService(time.Minute)
	_, _, err := svc.ParseAccessToken("not.a.jwt.at.all")
	if err != ErrInvalidToken {
		t.Fatalf("want ErrInvalidToken for malformed token, got %v", err)
	}
}

// TestJWTEmptyToken verifies that an empty string is rejected.
func TestJWTEmptyToken(t *testing.T) {
	svc := newTestService(time.Minute)
	_, _, err := svc.ParseAccessToken("")
	if err != ErrInvalidToken {
		t.Fatalf("want ErrInvalidToken for empty token, got %v", err)
	}
}

// TestJWTTamperedToken verifies that a token with a corrupted signature is rejected.
func TestJWTTamperedToken(t *testing.T) {
	svc := newTestService(time.Minute)
	token, _, err := svc.IssueAccess(uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		t.Fatal("unexpected JWT format")
	}
	sig := []byte(parts[2])
	sig[len(sig)-1] ^= 0xff
	tampered := parts[0] + "." + parts[1] + "." + string(sig)
	_, _, err = svc.ParseAccessToken(tampered)
	if err != ErrInvalidToken {
		t.Fatalf("want ErrInvalidToken for tampered token, got %v", err)
	}
}

// TestJWTDeviceSessionIDOptional verifies that a token with uuid.Nil sessionID
// is still parseable (backward compat with old tokens without "sid" claim).
func TestJWTDeviceSessionIDOptional(t *testing.T) {
	svc := newTestService(time.Minute)
	userID := uuid.New()
	token, _, err := svc.IssueAccess(userID, uuid.Nil)
	if err != nil {
		t.Fatalf("IssueAccess with nil session: %v", err)
	}
	gotUser, gotSession, err := svc.ParseAccessToken(token)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if gotUser != userID {
		t.Errorf("userID mismatch")
	}
	if gotSession != uuid.Nil {
		t.Errorf("sessionID: want Nil, got %v", gotSession)
	}
}
