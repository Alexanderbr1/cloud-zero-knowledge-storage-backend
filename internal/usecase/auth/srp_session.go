package auth

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	srppkg "cloud-backend/pkg/srp"
)

const srpSessionTTL = 5 * time.Minute

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

type srpSessionStore struct {
	mu   sync.Mutex
	data map[string]*srpSessEntry
}

// NewSRPSessionStore creates an in-memory SRP session store with TTL cleanup.
func NewSRPSessionStore(ctx context.Context) *srpSessionStore {
	st := &srpSessionStore{data: make(map[string]*srpSessEntry)}
	go st.cleanup(ctx)
	return st
}

func (s *srpSessionStore) store(id uuid.UUID, e *srpSessEntry) {
	s.mu.Lock()
	s.data[id.String()] = e
	s.mu.Unlock()
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
