package auth

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	srppkg "cloud-backend/pkg/srp"
)

const (
	srpSessionTTL     = 5 * time.Minute
	srpSessionMaxSize = 10_000 // prevent unbounded growth under load
)

type srpSessEntry struct {
	userID     uuid.UUID
	email      string
	srpSaltHex string // hex-encoded raw SRP salt bytes
	aHex       string // client's public ephemeral A (hex), stored from LoginInit
	session    *srppkg.ServerSession
	cryptoSalt []byte
	bcryptSalt string
	expiresAt  time.Time
}

// srpSessionManager is the interface for the in-memory SRP handshake store.
type srpSessionManager interface {
	store(id uuid.UUID, e *srpSessEntry) bool
	consume(id uuid.UUID) (*srpSessEntry, bool)
}

type srpSessionStore struct {
	mu   sync.Mutex
	data map[string]*srpSessEntry
}

// NewSRPSessionStore creates an in-memory SRP session store with TTL cleanup.
func NewSRPSessionStore(ctx context.Context) srpSessionManager {
	st := &srpSessionStore{data: make(map[string]*srpSessEntry)}
	go st.cleanup(ctx)
	return st
}

// store saves the entry and reports whether it was accepted.
// Returns false when the store is at capacity (safety valve against memory exhaustion).
func (s *srpSessionStore) store(id uuid.UUID, e *srpSessEntry) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data) >= srpSessionMaxSize {
		return false
	}
	s.data[id.String()] = e
	return true
}

// consume retrieves and removes the entry atomically; returns false if not found or expired.
func (s *srpSessionStore) consume(id uuid.UUID) (*srpSessEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := id.String()
	e, ok := s.data[key]
	if !ok {
		return nil, false
	}
	delete(s.data, key)
	if time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e, true
}

func (s *srpSessionStore) cleanup(ctx context.Context) {
	ticker := time.NewTicker(srpSessionTTL)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for k, e := range s.data {
				if now.After(e.expiresAt) {
					delete(s.data, k)
				}
			}
			s.mu.Unlock()
		}
	}
}
