package memory

import (
	"context"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	authuc "cloud-backend/internal/usecase/auth"
	srppkg "cloud-backend/pkg/srp"
)

func makeEntry(t *testing.T, ttl time.Duration) *authuc.SRPSessEntry {
	t.Helper()
	// v = g = 2 — minimal valid group element for a test verifier.
	sess, err := srppkg.NewServerSession(hex.EncodeToString([]byte{2}))
	if err != nil {
		t.Fatalf("NewServerSession: %v", err)
	}
	return &authuc.SRPSessEntry{
		UserID:             uuid.New(),
		Email:              "test@example.com",
		SRPSaltHex:         "deadbeef",
		AHex:               "cafebabe",
		Session:            sess,
		CryptoSalt:         []byte{1, 2, 3, 4},
		KEKEncryptedMaster: []byte("kek-master-bytes"),
		ExpiresAt:          time.Now().Add(ttl),
	}
}

func newStore(t *testing.T) authuc.SRPSessionManager {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return NewSRPSessionStore(ctx)
}

// TestMemoryStoreAndConsume verifies the happy path: store an entry and consume it.
func TestMemoryStoreAndConsume(t *testing.T) {
	store := newStore(t)
	id := uuid.New()
	entry := makeEntry(t, 5*time.Minute)

	if !store.Store(id, entry) {
		t.Fatal("Store returned false")
	}

	got, ok := store.Consume(id)
	if !ok {
		t.Fatal("Consume returned false")
	}
	if got.UserID != entry.UserID {
		t.Errorf("UserID mismatch: want %v, got %v", entry.UserID, got.UserID)
	}
	if string(got.KEKEncryptedMaster) != string(entry.KEKEncryptedMaster) {
		t.Errorf("KEKEncryptedMaster mismatch: want %q, got %q", entry.KEKEncryptedMaster, got.KEKEncryptedMaster)
	}
}

// TestMemoryConsumePreventReplay verifies that a session cannot be consumed twice.
func TestMemoryConsumePreventReplay(t *testing.T) {
	store := newStore(t)
	id := uuid.New()
	store.Store(id, makeEntry(t, 5*time.Minute))

	_, ok1 := store.Consume(id)
	_, ok2 := store.Consume(id)

	if !ok1 {
		t.Fatal("first Consume should succeed")
	}
	if ok2 {
		t.Fatal("second Consume should fail (replay protection)")
	}
}

// TestMemoryExpiredSession verifies that an already-expired session is rejected.
func TestMemoryExpiredSession(t *testing.T) {
	store := newStore(t)
	id := uuid.New()
	store.Store(id, makeEntry(t, -time.Second))

	_, ok := store.Consume(id)
	if ok {
		t.Fatal("expired session should not be consumable")
	}
}

// TestMemoryConsumeUnknownID verifies that Consume returns false for an unknown ID.
func TestMemoryConsumeUnknownID(t *testing.T) {
	store := newStore(t)
	_, ok := store.Consume(uuid.New())
	if ok {
		t.Fatal("Consume of unknown ID should return false")
	}
}

// TestMemoryCapacityLimit verifies that Store rejects entries beyond the capacity.
// Reuses a single entry template to avoid 10 000 crypto operations.
func TestMemoryCapacityLimit(t *testing.T) {
	store := newStore(t)
	template := makeEntry(t, 5*time.Minute)

	for i := 0; i < srpSessionMaxSize; i++ {
		if !store.Store(uuid.New(), template) {
			t.Fatalf("Store failed before capacity at i=%d", i)
		}
	}

	if store.Store(uuid.New(), template) {
		t.Fatal("Store should return false when at capacity")
	}
}

// TestMemoryConcurrentStoreConsume verifies that concurrent Store and Consume
// calls do not race or corrupt internal state.
func TestMemoryConcurrentStoreConsume(t *testing.T) {
	store := newStore(t)
	const n = 200
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
	}
	entry := makeEntry(t, 5*time.Minute)

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			store.Store(id, entry)
		}(id)
	}
	wg.Wait()

	var mu sync.Mutex
	consumed := 0
	for _, id := range ids {
		wg.Add(1)
		go func(id uuid.UUID) {
			defer wg.Done()
			if _, ok := store.Consume(id); ok {
				mu.Lock()
				consumed++
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()

	if consumed != n {
		t.Errorf("want %d consumed, got %d", n, consumed)
	}
}
