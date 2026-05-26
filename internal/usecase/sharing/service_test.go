package sharing_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/internal/entity"
	sharing "cloud-backend/internal/usecase/sharing"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func mustURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

// activeShare builds a non-expired, non-revoked FileShareView for the given recipient.
func activeShare(recipientID, ownerID uuid.UUID) entity.FileShareView {
	return entity.FileShareView{
		FileShare: entity.FileShare{
			ID:          uuid.New(),
			BlobID:      uuid.New(),
			OwnerID:     ownerID,
			RecipientID: recipientID,
		},
		OwnerEmail:     "owner@example.com",
		RecipientEmail: "recip@example.com",
		BlobFileName:   "photo.jpg",
	}
}

// ─── mockShareRepository ─────────────────────────────────────────────────────

type mockShareRepo struct {
	created        entity.FileShareView
	createErr      error
	share          entity.FileShareView
	shareFound     bool
	shareErr       error
	sharedWithUser []entity.FileShareView
	listWithErr    error
	sharesForBlob  []entity.FileShareView
	listForErr     error
	revokeErr      error
}

func (m *mockShareRepo) CreateShare(_ context.Context, _ sharing.CreateShareParams) (entity.FileShareView, error) {
	return m.created, m.createErr
}
func (m *mockShareRepo) GetShare(_ context.Context, _ uuid.UUID) (entity.FileShareView, bool, error) {
	return m.share, m.shareFound, m.shareErr
}
func (m *mockShareRepo) ListSharedWithUser(_ context.Context, _ uuid.UUID) ([]entity.FileShareView, error) {
	return m.sharedWithUser, m.listWithErr
}
func (m *mockShareRepo) ListSharesForBlob(_ context.Context, _, _ uuid.UUID) ([]entity.FileShareView, error) {
	return m.sharesForBlob, m.listForErr
}
func (m *mockShareRepo) RevokeShare(_ context.Context, _, _ uuid.UUID) error {
	return m.revokeErr
}

// ─── mockUserKeyStore ─────────────────────────────────────────────────────────

type mockUserKeyStore struct {
	publicKey []byte
	userID    uuid.UUID
	err       error
}

func (m *mockUserKeyStore) GetPublicKeyByEmail(_ context.Context, _ string) ([]byte, uuid.UUID, error) {
	return m.publicKey, m.userID, m.err
}

// ─── mockBlobStore ────────────────────────────────────────────────────────────

type mockBlobStore struct {
	info      sharing.BlobInfo
	infoFound bool
	infoErr   error
}

func (m *mockBlobStore) GetBlobInfo(_ context.Context, _ uuid.UUID) (sharing.BlobInfo, bool, error) {
	return m.info, m.infoFound, m.infoErr
}

// ─── mockObjectSigner ─────────────────────────────────────────────────────────

type mockObjectSigner struct {
	presignURL *url.URL
	presignErr error
}

func (m *mockObjectSigner) PresignedGetObject(_ context.Context, _ string, _ time.Duration) (*url.URL, error) {
	if m.presignErr != nil {
		return nil, m.presignErr
	}
	if m.presignURL != nil {
		return m.presignURL, nil
	}
	return mustURL("https://s3.example.com/file"), nil
}

// ─── mockNotifier ─────────────────────────────────────────────────────────────

type mockNotifier struct {
	shareErr error
}

func (m *mockNotifier) NotifyNewShare(_ context.Context, _, _, _ string) error {
	return m.shareErr
}

// ─── fixture ─────────────────────────────────────────────────────────────────

type sharingFixture struct {
	shares   *mockShareRepo
	users    *mockUserKeyStore
	blobs    *mockBlobStore
	objects  *mockObjectSigner
	notifier *mockNotifier
	svc      *sharing.Service
}

func newFixture() *sharingFixture {
	f := &sharingFixture{
		shares:   &mockShareRepo{},
		users:    &mockUserKeyStore{},
		blobs:    &mockBlobStore{},
		objects:  &mockObjectSigner{},
		notifier: &mockNotifier{},
	}
	f.svc = &sharing.Service{
		Shares:     f.shares,
		Users:      f.users,
		Blobs:      f.blobs,
		Objects:    f.objects,
		PresignTTL: time.Minute,
		Notifier:   f.notifier,
		Logger:     zerolog.Nop(),
	}
	return f
}

// ─── GetRecipientPublicKey ────────────────────────────────────────────────────

func TestGetRecipientPublicKey_HappyPath(t *testing.T) {
	f := newFixture()
	f.users.publicKey = []byte("spki-bytes")

	key, err := f.svc.GetRecipientPublicKey(context.Background(), "Alice@Example.COM")
	if err != nil {
		t.Fatalf("GetRecipientPublicKey: %v", err)
	}
	if string(key) != "spki-bytes" {
		t.Errorf("want spki-bytes, got %s", key)
	}
}

func TestGetRecipientPublicKey_RepoError(t *testing.T) {
	f := newFixture()
	f.users.err = errors.New("db error")

	_, err := f.svc.GetRecipientPublicKey(context.Background(), "alice@example.com")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── CreateShare ──────────────────────────────────────────────────────────────

func TestCreateShare_HappyPath(t *testing.T) {
	f := newFixture()
	ownerID := uuid.New()
	recipientID := uuid.New()

	f.blobs.info = sharing.BlobInfo{OwnerID: ownerID, ObjectKey: "u/blob"}
	f.blobs.infoFound = true
	f.users.userID = recipientID
	f.users.publicKey = []byte("pub")
	f.shares.created = activeShare(recipientID, ownerID)

	share, err := f.svc.CreateShare(context.Background(), sharing.CreateShareParams{
		BlobID:         uuid.New(),
		OwnerID:        ownerID,
		RecipientEmail: "recip@example.com",
		EphemeralPub:   []byte("ephem"),
		WrappedFileKey: []byte("key"),
	})
	if err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	if share.RecipientID != recipientID {
		t.Errorf("RecipientID mismatch")
	}
}

func TestCreateShare_BlobNotFound(t *testing.T) {
	f := newFixture()
	f.blobs.infoFound = false

	_, err := f.svc.CreateShare(context.Background(), sharing.CreateShareParams{
		BlobID:         uuid.New(),
		OwnerID:        uuid.New(),
		RecipientEmail: "recip@example.com",
	})
	if !errors.Is(err, sharing.ErrNotFound) {
		t.Fatalf("want ErrNotFound for missing blob, got %v", err)
	}
}

func TestCreateShare_CallerIsNotOwner(t *testing.T) {
	f := newFixture()
	f.blobs.infoFound = true
	f.blobs.info = sharing.BlobInfo{OwnerID: uuid.New()} // owner is someone else

	_, err := f.svc.CreateShare(context.Background(), sharing.CreateShareParams{
		BlobID:         uuid.New(),
		OwnerID:        uuid.New(), // different from blob.OwnerID
		RecipientEmail: "recip@example.com",
	})
	if !errors.Is(err, sharing.ErrNotFound) {
		t.Fatalf("want ErrNotFound when caller is not owner, got %v", err)
	}
}

func TestCreateShare_SelfShare(t *testing.T) {
	f := newFixture()
	ownerID := uuid.New()
	f.blobs.infoFound = true
	f.blobs.info = sharing.BlobInfo{OwnerID: ownerID}
	f.users.userID = ownerID // recipient resolves to the same user

	_, err := f.svc.CreateShare(context.Background(), sharing.CreateShareParams{
		BlobID:         uuid.New(),
		OwnerID:        ownerID,
		RecipientEmail: "owner@example.com",
	})
	if !errors.Is(err, sharing.ErrSelfShare) {
		t.Fatalf("want ErrSelfShare, got %v", err)
	}
}

func TestCreateShare_RecipientLookupError(t *testing.T) {
	f := newFixture()
	ownerID := uuid.New()
	f.blobs.infoFound = true
	f.blobs.info = sharing.BlobInfo{OwnerID: ownerID}
	f.users.err = errors.New("user not found")

	_, err := f.svc.CreateShare(context.Background(), sharing.CreateShareParams{
		BlobID:         uuid.New(),
		OwnerID:        ownerID,
		RecipientEmail: "ghost@example.com",
	})
	if err == nil {
		t.Fatal("expected error when recipient lookup fails")
	}
}

func TestCreateShare_RepoError(t *testing.T) {
	f := newFixture()
	ownerID := uuid.New()
	recipientID := uuid.New()
	f.blobs.infoFound = true
	f.blobs.info = sharing.BlobInfo{OwnerID: ownerID}
	f.users.userID = recipientID
	f.shares.createErr = errors.New("unique constraint violated")

	_, err := f.svc.CreateShare(context.Background(), sharing.CreateShareParams{
		BlobID:         uuid.New(),
		OwnerID:        ownerID,
		RecipientEmail: "recip@example.com",
	})
	if err == nil {
		t.Fatal("expected error from share repo")
	}
}

// ─── GetSharedFile ────────────────────────────────────────────────────────────

func TestGetSharedFile_HappyPath(t *testing.T) {
	f := newFixture()
	callerID := uuid.New()
	ownerID := uuid.New()
	share := activeShare(callerID, ownerID)

	f.shares.share = share
	f.shares.shareFound = true
	f.blobs.info = sharing.BlobInfo{
		ObjectKey:   "u/blob",
		FileName:    "photo.jpg",
		ContentType: "image/jpeg",
		FileIV:      []byte("iv"),
	}
	f.blobs.infoFound = true

	result, err := f.svc.GetSharedFile(context.Background(), share.ID, callerID)
	if err != nil {
		t.Fatalf("GetSharedFile: %v", err)
	}
	if result.DownloadURL == "" {
		t.Error("expected non-empty download URL")
	}
	if result.FileName != "photo.jpg" {
		t.Errorf("FileName: want photo.jpg, got %s", result.FileName)
	}
}

func TestGetSharedFile_ShareNotFound(t *testing.T) {
	f := newFixture()
	f.shares.shareFound = false

	_, err := f.svc.GetSharedFile(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, sharing.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGetSharedFile_WrongCaller(t *testing.T) {
	f := newFixture()
	share := activeShare(uuid.New(), uuid.New())
	f.shares.share = share
	f.shares.shareFound = true

	_, err := f.svc.GetSharedFile(context.Background(), share.ID, uuid.New()) // different caller
	if !errors.Is(err, sharing.ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}

func TestGetSharedFile_Revoked(t *testing.T) {
	f := newFixture()
	callerID := uuid.New()
	share := activeShare(callerID, uuid.New())
	now := time.Now()
	share.RevokedAt = &now
	f.shares.share = share
	f.shares.shareFound = true

	_, err := f.svc.GetSharedFile(context.Background(), share.ID, callerID)
	if !errors.Is(err, sharing.ErrRevoked) {
		t.Fatalf("want ErrRevoked, got %v", err)
	}
}

func TestGetSharedFile_Expired(t *testing.T) {
	f := newFixture()
	callerID := uuid.New()
	share := activeShare(callerID, uuid.New())
	past := time.Now().Add(-time.Hour)
	share.ExpiresAt = &past
	f.shares.share = share
	f.shares.shareFound = true

	_, err := f.svc.GetSharedFile(context.Background(), share.ID, callerID)
	if !errors.Is(err, sharing.ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestGetSharedFile_FutureExpiry_Allowed(t *testing.T) {
	f := newFixture()
	callerID := uuid.New()
	share := activeShare(callerID, uuid.New())
	future := time.Now().Add(time.Hour)
	share.ExpiresAt = &future
	f.shares.share = share
	f.shares.shareFound = true
	f.blobs.infoFound = true

	_, err := f.svc.GetSharedFile(context.Background(), share.ID, callerID)
	if errors.Is(err, sharing.ErrExpired) {
		t.Fatal("share with future expiry should be accessible")
	}
}

func TestGetSharedFile_BlobDisappeared(t *testing.T) {
	f := newFixture()
	callerID := uuid.New()
	share := activeShare(callerID, uuid.New())
	f.shares.share = share
	f.shares.shareFound = true
	f.blobs.infoFound = false // blob was hard-deleted after share was created

	_, err := f.svc.GetSharedFile(context.Background(), share.ID, callerID)
	if !errors.Is(err, sharing.ErrNotFound) {
		t.Fatalf("want ErrNotFound when blob no longer exists, got %v", err)
	}
}

func TestGetSharedFile_PresignError(t *testing.T) {
	f := newFixture()
	callerID := uuid.New()
	share := activeShare(callerID, uuid.New())
	f.shares.share = share
	f.shares.shareFound = true
	f.blobs.infoFound = true
	f.objects.presignErr = errors.New("s3 unavailable")

	_, err := f.svc.GetSharedFile(context.Background(), share.ID, callerID)
	if err == nil {
		t.Fatal("expected error when presign fails")
	}
}

// ─── ListSharedWithMe ─────────────────────────────────────────────────────────

func TestListSharedWithMe_HappyPath(t *testing.T) {
	f := newFixture()
	userID := uuid.New()
	f.shares.sharedWithUser = []entity.FileShareView{
		activeShare(userID, uuid.New()),
		activeShare(userID, uuid.New()),
	}

	result, err := f.svc.ListSharedWithMe(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListSharedWithMe: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("want 2 shares, got %d", len(result))
	}
}

func TestListSharedWithMe_RepoError(t *testing.T) {
	f := newFixture()
	f.shares.listWithErr = errors.New("db error")

	_, err := f.svc.ListSharedWithMe(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── ListMyShares ─────────────────────────────────────────────────────────────

func TestListMyShares_HappyPath(t *testing.T) {
	f := newFixture()
	ownerID := uuid.New()
	f.shares.sharesForBlob = []entity.FileShareView{
		activeShare(uuid.New(), ownerID),
	}

	result, err := f.svc.ListMyShares(context.Background(), uuid.New(), ownerID)
	if err != nil {
		t.Fatalf("ListMyShares: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("want 1 share, got %d", len(result))
	}
}

func TestListMyShares_RepoError(t *testing.T) {
	f := newFixture()
	f.shares.listForErr = errors.New("db error")

	_, err := f.svc.ListMyShares(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── RevokeShare ─────────────────────────────────────────────────────────────

func TestRevokeShare_HappyPath(t *testing.T) {
	f := newFixture()

	if err := f.svc.RevokeShare(context.Background(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("RevokeShare: %v", err)
	}
}

func TestRevokeShare_RepoError(t *testing.T) {
	f := newFixture()
	f.shares.revokeErr = errors.New("not found or not owner")

	err := f.svc.RevokeShare(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error from repo")
	}
}
