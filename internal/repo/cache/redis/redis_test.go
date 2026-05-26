package redis

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	authuc "cloud-backend/internal/usecase/auth"
	srppkg "cloud-backend/pkg/srp"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func newTestClient(t *testing.T) *goredis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	return goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
}

// newTestClientWithServer returns the miniredis server as well so tests can
// manipulate TTLs with mr.FastForward.
func newTestClientWithServer(t *testing.T) (*goredis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	return client, mr
}

func makeEntry(t *testing.T, ttl time.Duration) *authuc.SRPSessEntry {
	t.Helper()
	sess, err := srppkg.NewServerSession(hex.EncodeToString([]byte{2}))
	if err != nil {
		t.Fatalf("NewServerSession: %v", err)
	}
	return &authuc.SRPSessEntry{
		UserID:              uuid.New(),
		Email:               "alice@example.com",
		SRPSaltHex:          "deadbeef",
		AHex:                "cafebabe",
		Session:             sess,
		CryptoSalt:          []byte{1, 2, 3, 4},
		BcryptSalt:          "$2a$12$testsalt",
		EncryptedPrivateKey: []byte("encrypted-private-key-bytes"),
		KEKEncryptedMaster:  []byte("kek-encrypted-master-bytes"),
		ExpiresAt:           time.Now().Add(ttl),
	}
}

// ─── SRP session store ────────────────────────────────────────────────────────

func TestRedisSRPSession_StoreAndConsume(t *testing.T) {
	store := NewSRPSessionStore(newTestClient(t))
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
		t.Errorf("UserID: want %v, got %v", entry.UserID, got.UserID)
	}
	if got.Email != entry.Email {
		t.Errorf("Email: want %q, got %q", entry.Email, got.Email)
	}
	if got.SRPSaltHex != entry.SRPSaltHex {
		t.Errorf("SRPSaltHex: want %q, got %q", entry.SRPSaltHex, got.SRPSaltHex)
	}
	if got.BcryptSalt != entry.BcryptSalt {
		t.Errorf("BcryptSalt: want %q, got %q", entry.BcryptSalt, got.BcryptSalt)
	}
	if string(got.CryptoSalt) != string(entry.CryptoSalt) {
		t.Errorf("CryptoSalt: want %v, got %v", entry.CryptoSalt, got.CryptoSalt)
	}
	if string(got.EncryptedPrivateKey) != string(entry.EncryptedPrivateKey) {
		t.Errorf("EncryptedPrivateKey: want %v, got %v", entry.EncryptedPrivateKey, got.EncryptedPrivateKey)
	}
	// KEKEncryptedMaster is the field that was missing before the bug fix —
	// verify it survives the Redis round-trip.
	if string(got.KEKEncryptedMaster) != string(entry.KEKEncryptedMaster) {
		t.Errorf("KEKEncryptedMaster: want %v, got %v", entry.KEKEncryptedMaster, got.KEKEncryptedMaster)
	}
	if got.Session == nil {
		t.Error("Session must not be nil after restore")
	}
	if got.Session.PublicEphemeralHex() != entry.Session.PublicEphemeralHex() {
		t.Error("Session B mismatch after restore")
	}
}

func TestRedisSRPSession_ConsumePreventReplay(t *testing.T) {
	store := NewSRPSessionStore(newTestClient(t))
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

func TestRedisSRPSession_ConsumeUnknownID(t *testing.T) {
	store := NewSRPSessionStore(newTestClient(t))

	_, ok := store.Consume(uuid.New())
	if ok {
		t.Fatal("Consume of unknown ID should return false")
	}
}

func TestRedisSRPSession_ExpiredEntry(t *testing.T) {
	client, mr := newTestClientWithServer(t)
	store := NewSRPSessionStore(client)
	id := uuid.New()

	store.Store(id, makeEntry(t, 5*time.Minute))
	mr.FastForward(6 * time.Minute) // advance Redis TTL past expiry

	_, ok := store.Consume(id)
	if ok {
		t.Fatal("expired session should not be consumable")
	}
}

func TestRedisSRPSession_StoreExpiredEntry_ReturnsFalse(t *testing.T) {
	store := NewSRPSessionStore(newTestClient(t))

	// Negative TTL means already expired — Store must reject it.
	ok := store.Store(uuid.New(), makeEntry(t, -time.Second))
	if ok {
		t.Fatal("Store should return false for an already-expired entry")
	}
}

// ─── Session blocklist ────────────────────────────────────────────────────────

func TestBlocklist_BlockAndIsBlocked(t *testing.T) {
	bl := NewSessionBlocklist(newTestClient(t))
	id := uuid.New()
	ctx := context.Background()

	blocked, err := bl.IsBlocked(ctx, id)
	if err != nil || blocked {
		t.Fatalf("want (false, nil) before block, got (%v, %v)", blocked, err)
	}

	if err := bl.Block(ctx, id, time.Minute); err != nil {
		t.Fatalf("Block: %v", err)
	}

	blocked, err = bl.IsBlocked(ctx, id)
	if err != nil {
		t.Fatalf("IsBlocked: %v", err)
	}
	if !blocked {
		t.Error("session should be blocked after Block")
	}
}

func TestBlocklist_TTLExpiry(t *testing.T) {
	client, mr := newTestClientWithServer(t)
	bl := NewSessionBlocklist(client)
	id := uuid.New()
	ctx := context.Background()

	bl.Block(ctx, id, time.Minute)
	mr.FastForward(2 * time.Minute)

	blocked, err := bl.IsBlocked(ctx, id)
	if err != nil {
		t.Fatalf("IsBlocked after expiry: %v", err)
	}
	if blocked {
		t.Error("session should no longer be blocked after TTL expires")
	}
}

func TestBlocklist_BlockBatch(t *testing.T) {
	bl := NewSessionBlocklist(newTestClient(t))
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	ctx := context.Background()

	if err := bl.BlockBatch(ctx, ids, time.Minute); err != nil {
		t.Fatalf("BlockBatch: %v", err)
	}

	for _, id := range ids {
		blocked, err := bl.IsBlocked(ctx, id)
		if err != nil {
			t.Errorf("IsBlocked(%v): %v", id, err)
		}
		if !blocked {
			t.Errorf("session %v should be blocked after BlockBatch", id)
		}
	}
}

func TestBlocklist_BlockBatch_Empty_NoError(t *testing.T) {
	bl := NewSessionBlocklist(newTestClient(t))

	if err := bl.BlockBatch(context.Background(), nil, time.Minute); err != nil {
		t.Fatalf("BlockBatch with empty slice should be a no-op, got: %v", err)
	}
}

// ─── Rate limiter ─────────────────────────────────────────────────────────────

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewRateLimiter(newTestClient(t), "rl:", 5, time.Minute)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		allowed, err := rl.Allow(ctx, "key1")
		if err != nil {
			t.Fatalf("Allow[%d]: %v", i, err)
		}
		if !allowed {
			t.Fatalf("request %d of 5 should be allowed", i+1)
		}
	}
}

func TestRateLimiter_DeniesAtLimit(t *testing.T) {
	rl := NewRateLimiter(newTestClient(t), "rl:", 3, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		rl.Allow(ctx, "key2") //nolint
	}

	allowed, err := rl.Allow(ctx, "key2")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if allowed {
		t.Error("4th request should be denied when limit is 3")
	}
}

func TestRateLimiter_WindowReset(t *testing.T) {
	client, mr := newTestClientWithServer(t)
	rl := NewRateLimiter(client, "rl:", 2, time.Minute)
	ctx := context.Background()

	rl.Allow(ctx, "key3") //nolint
	rl.Allow(ctx, "key3") //nolint

	// 3rd request in same window — denied.
	allowed, _ := rl.Allow(ctx, "key3")
	if allowed {
		t.Fatal("3rd request should be denied within window")
	}

	// Advance past the window — counter resets.
	mr.FastForward(2 * time.Minute)

	allowed, err := rl.Allow(ctx, "key3")
	if err != nil {
		t.Fatalf("Allow after window reset: %v", err)
	}
	if !allowed {
		t.Error("first request after window reset should be allowed")
	}
}

func TestRateLimiter_DifferentKeys_Independent(t *testing.T) {
	rl := NewRateLimiter(newTestClient(t), "rl:", 1, time.Minute)
	ctx := context.Background()

	rl.Allow(ctx, "userA") //nolint — exhaust userA's quota

	// userB's counter is independent.
	allowed, err := rl.Allow(ctx, "userB")
	if err != nil {
		t.Fatalf("Allow userB: %v", err)
	}
	if !allowed {
		t.Error("userB should not be affected by userA's quota")
	}
}
