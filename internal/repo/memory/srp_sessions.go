package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	authuc "cloud-backend/internal/usecase/auth"
)

const srpSessionMaxSize = 10_000

type srpSessionStore struct {
	mu   sync.Mutex
	data map[string]*authuc.SRPSessEntry
}

// NewSRPSessionStore returns a thread-safe in-memory SRP handshake store with
// background TTL cleanup tied to ctx.
func NewSRPSessionStore(ctx context.Context) authuc.SRPSessionManager {
	st := &srpSessionStore{data: make(map[string]*authuc.SRPSessEntry)}
	go st.cleanup(ctx)
	return st
}

func (s *srpSessionStore) Store(id uuid.UUID, e *authuc.SRPSessEntry) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.data) >= srpSessionMaxSize {
		return false
	}
	s.data[id.String()] = e
	return true
}

func (s *srpSessionStore) Consume(id uuid.UUID) (*authuc.SRPSessEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := id.String()
	e, ok := s.data[key]
	if !ok {
		return nil, false
	}
	delete(s.data, key)
	if time.Now().After(e.ExpiresAt) {
		return nil, false
	}
	return e, true
}

func (s *srpSessionStore) cleanup(ctx context.Context) {
	ticker := time.NewTicker(authuc.SRPSessionTTL)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for k, e := range s.data {
				if now.After(e.ExpiresAt) {
					delete(s.data, k)
				}
			}
			s.mu.Unlock()
		}
	}
}
