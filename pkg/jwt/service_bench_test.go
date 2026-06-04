package jwt

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

var benchSvc = NewService([]byte("bench-secret-32-bytes-1234567890"), 15*time.Minute)

// BenchmarkIssueAccess measures JWT signing — called on every login and token refresh.
func BenchmarkIssueAccess(b *testing.B) {
	userID := uuid.New()
	sessionID := uuid.New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := benchSvc.IssueAccess(userID, sessionID); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseAccessToken measures JWT verification — called on every authenticated request.
func BenchmarkParseAccessToken(b *testing.B) {
	token, _, err := benchSvc.IssueAccess(uuid.New(), uuid.New())
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := benchSvc.ParseAccessToken(token); err != nil {
			b.Fatal(err)
		}
	}
}
