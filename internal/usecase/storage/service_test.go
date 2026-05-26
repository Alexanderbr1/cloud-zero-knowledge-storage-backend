package storage_test

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
	storage "cloud-backend/internal/usecase/storage"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func ptr[T any](v T) *T { return &v }

// ─── mockObjectStore ──────────────────────────────────────────────────────────

type mockObjectStore struct {
	mu            sync.Mutex
	removedKeys   []string
	presignPutURL *url.URL
	presignPutErr error
	presignGetURL *url.URL
	presignGetErr error
	removeErr     error
	statSize      int64
	statErr       error
}

func (m *mockObjectStore) EnsureBucket(_ context.Context) error { return nil }
func (m *mockObjectStore) PresignedPutObject(_ context.Context, _ string, _ time.Duration) (*url.URL, error) {
	if m.presignPutErr != nil {
		return nil, m.presignPutErr
	}
	if m.presignPutURL != nil {
		return m.presignPutURL, nil
	}
	return mustURL("https://s3.example.com/upload"), nil
}
func (m *mockObjectStore) PresignedGetObject(_ context.Context, _ string, _ time.Duration) (*url.URL, error) {
	if m.presignGetErr != nil {
		return nil, m.presignGetErr
	}
	if m.presignGetURL != nil {
		return m.presignGetURL, nil
	}
	return mustURL("https://s3.example.com/download"), nil
}
func (m *mockObjectStore) RemoveObject(_ context.Context, key string) error {
	m.mu.Lock()
	m.removedKeys = append(m.removedKeys, key)
	m.mu.Unlock()
	return m.removeErr
}
func (m *mockObjectStore) StatObject(_ context.Context, _ string) (int64, error) {
	return m.statSize, m.statErr
}

// ─── mockBlobRepo ─────────────────────────────────────────────────────────────

type mockBlobRepo struct {
	storageUsed        int64
	storageUsedErr     error
	registerErr        error
	pending            storage.PendingBlobMeta
	pendingFound       bool
	pendingErr         error
	confirmOk          bool
	confirmErr         error
	blobMeta           storage.BlobMeta
	blobMetaFound      bool
	blobMetaErr        error
	trashName          string
	trashOk            bool
	trashErr           error
	moveInfo           storage.MoveBlobInfo
	moveOk             bool
	moveErr            error
	renameOk           bool
	renameErr          error
	searchBlobsResult  []storage.SearchBlobRecord
	searchBlobsErr     error
	purgeOrphanedKeys  []string
	purgeOrphanedErr   error
	listInFolderResult []entity.Blob
	listInFolderErr    error
}

func (m *mockBlobRepo) GetUserStorageUsed(_ context.Context, _ uuid.UUID) (int64, error) {
	return m.storageUsed, m.storageUsedErr
}
func (m *mockBlobRepo) RegisterBlob(_ context.Context, _ storage.RegisterBlobParams) error {
	return m.registerErr
}
func (m *mockBlobRepo) GetPendingBlobMeta(_ context.Context, _, _ uuid.UUID) (storage.PendingBlobMeta, bool, error) {
	return m.pending, m.pendingFound, m.pendingErr
}
func (m *mockBlobRepo) ConfirmBlobUploadWithSize(_ context.Context, _, _ uuid.UUID, _ int64) (bool, error) {
	return m.confirmOk, m.confirmErr
}
func (m *mockBlobRepo) PurgeOrphanedBlobs(_ context.Context, _ time.Time) ([]string, error) {
	return m.purgeOrphanedKeys, m.purgeOrphanedErr
}
func (m *mockBlobRepo) GetBlobMeta(_ context.Context, _, _ uuid.UUID) (storage.BlobMeta, bool, error) {
	return m.blobMeta, m.blobMetaFound, m.blobMetaErr
}
func (m *mockBlobRepo) RemoveBlob(_ context.Context, _, _ uuid.UUID) (string, bool, error) {
	return "", false, nil
}
func (m *mockBlobRepo) ListBlobs(_ context.Context, _ uuid.UUID) ([]entity.Blob, error) {
	return nil, nil
}
func (m *mockBlobRepo) ListBlobsInFolder(_ context.Context, _ uuid.UUID, _ *uuid.UUID) ([]entity.Blob, error) {
	return m.listInFolderResult, m.listInFolderErr
}
func (m *mockBlobRepo) MoveBlob(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID) (storage.MoveBlobInfo, bool, error) {
	return m.moveInfo, m.moveOk, m.moveErr
}
func (m *mockBlobRepo) RenameBlob(_ context.Context, _, _ uuid.UUID, _ string) (bool, error) {
	return m.renameOk, m.renameErr
}
func (m *mockBlobRepo) SearchBlobs(_ context.Context, _ uuid.UUID, _ string) ([]storage.SearchBlobRecord, error) {
	return m.searchBlobsResult, m.searchBlobsErr
}
func (m *mockBlobRepo) TrashBlob(_ context.Context, _, _ uuid.UUID) (string, bool, error) {
	return m.trashName, m.trashOk, m.trashErr
}

// ─── mockBlobTrashRepo ────────────────────────────────────────────────────────

type mockBlobTrashRepo struct {
	restoreName      string
	restoreOk        bool
	restoreErr       error
	hardDeleteKey    string
	hardDeleteName   string
	hardDeleteOk     bool
	hardDeleteErr    error
	listedBlobs      []entity.Blob
	listErr          error
	emptiedBlobs     []storage.EmptiedBlob
	emptyErr         error
	purgeExpiredKeys []string
	purgeExpiredErr  error
}

func (m *mockBlobTrashRepo) RestoreBlob(_ context.Context, _, _ uuid.UUID) (string, bool, error) {
	return m.restoreName, m.restoreOk, m.restoreErr
}
func (m *mockBlobTrashRepo) HardDeleteBlob(_ context.Context, _, _ uuid.UUID) (string, string, bool, error) {
	return m.hardDeleteKey, m.hardDeleteName, m.hardDeleteOk, m.hardDeleteErr
}
func (m *mockBlobTrashRepo) ListTrashedBlobs(_ context.Context, _ uuid.UUID) ([]entity.Blob, error) {
	return m.listedBlobs, m.listErr
}
func (m *mockBlobTrashRepo) EmptyTrashedBlobs(_ context.Context, _ uuid.UUID) ([]storage.EmptiedBlob, error) {
	return m.emptiedBlobs, m.emptyErr
}
func (m *mockBlobTrashRepo) PurgeExpiredBlobs(_ context.Context, _ time.Time) ([]string, error) {
	return m.purgeExpiredKeys, m.purgeExpiredErr
}

// ─── mockFolderRepo ───────────────────────────────────────────────────────────

type mockFolderRepo struct {
	// getFolderFn overrides the default single-value behaviour when set —
	// necessary for MoveFolder tests that call GetFolder with different IDs.
	getFolderFn         func(ctx context.Context, folderID, userID uuid.UUID) (entity.Folder, bool, error)
	folder              entity.Folder
	folderOk            bool
	folderErr           error
	createFolder        entity.Folder
	createErr           error
	moveFolderErr       error
	isDescendant        bool
	isDescErr           error
	trashOk             bool
	trashErr            error
	renameFolderResult  entity.Folder
	renameFolderErr     error
	listFoldersResult   []entity.Folder
	listFoldersErr      error
	searchFoldersResult []entity.Folder
	searchFoldersErr    error
}

func (m *mockFolderRepo) GetFolder(ctx context.Context, folderID, userID uuid.UUID) (entity.Folder, bool, error) {
	if m.getFolderFn != nil {
		return m.getFolderFn(ctx, folderID, userID)
	}
	return m.folder, m.folderOk, m.folderErr
}
func (m *mockFolderRepo) CreateFolder(_ context.Context, p storage.CreateFolderParams) (entity.Folder, error) {
	return m.createFolder, m.createErr
}
func (m *mockFolderRepo) ListFolders(_ context.Context, _ uuid.UUID, _ *uuid.UUID) ([]entity.Folder, error) {
	return m.listFoldersResult, m.listFoldersErr
}
func (m *mockFolderRepo) RenameFolder(_ context.Context, _, _ uuid.UUID, _ string) (entity.Folder, error) {
	return m.renameFolderResult, m.renameFolderErr
}
func (m *mockFolderRepo) MoveFolder(_ context.Context, _ storage.MoveFolderParams) error {
	return m.moveFolderErr
}
func (m *mockFolderRepo) IsDescendantOf(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return m.isDescendant, m.isDescErr
}
func (m *mockFolderRepo) SearchFolders(_ context.Context, _ uuid.UUID, _ string) ([]entity.Folder, error) {
	return m.searchFoldersResult, m.searchFoldersErr
}
func (m *mockFolderRepo) TrashFolder(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return m.trashOk, m.trashErr
}

// ─── mockFolderTrashRepo ──────────────────────────────────────────────────────

type mockFolderTrashRepo struct {
	restoreName      string
	restoreOk        bool
	restoreErr       error
	hardDeleteKeys   []string
	hardDeleteName   string
	hardDeleteOk     bool
	hardDeleteErr    error
	listedFolders    []entity.Folder
	listErr          error
	emptiedFolders   []storage.EmptiedFolder
	emptiedKeys      []string
	emptyErr         error
	purgeExpiredKeys []string
	purgeExpiredErr  error
}

func (m *mockFolderTrashRepo) RestoreFolder(_ context.Context, _, _ uuid.UUID) (string, bool, error) {
	return m.restoreName, m.restoreOk, m.restoreErr
}
func (m *mockFolderTrashRepo) HardDeleteFolder(_ context.Context, _, _ uuid.UUID) ([]string, string, bool, error) {
	return m.hardDeleteKeys, m.hardDeleteName, m.hardDeleteOk, m.hardDeleteErr
}
func (m *mockFolderTrashRepo) ListTrashedFolders(_ context.Context, _ uuid.UUID) ([]entity.Folder, error) {
	return m.listedFolders, m.listErr
}
func (m *mockFolderTrashRepo) EmptyTrashedFolders(_ context.Context, _ uuid.UUID) ([]storage.EmptiedFolder, []string, error) {
	return m.emptiedFolders, m.emptiedKeys, m.emptyErr
}
func (m *mockFolderTrashRepo) PurgeExpiredFolders(_ context.Context, _ time.Time) ([]string, error) {
	return m.purgeExpiredKeys, m.purgeExpiredErr
}

// ─── fixture ─────────────────────────────────────────────────────────────────

type storageFixture struct {
	objects     *mockObjectStore
	blobs       *mockBlobRepo
	blobTrash   *mockBlobTrashRepo
	folders     *mockFolderRepo
	folderTrash *mockFolderTrashRepo
	svc         *storage.Service
}

func newFixture() *storageFixture {
	f := &storageFixture{
		objects:     &mockObjectStore{},
		blobs:       &mockBlobRepo{},
		blobTrash:   &mockBlobTrashRepo{},
		folders:     &mockFolderRepo{},
		folderTrash: &mockFolderTrashRepo{},
	}
	f.svc = &storage.Service{
		Objects:     f.objects,
		Blobs:       f.blobs,
		BlobTrash:   f.blobTrash,
		Folders:     f.folders,
		FolderTrash: f.folderTrash,
		PresignTTL:  time.Minute,
	}
	return f
}

// ─── GetStorageUsage ──────────────────────────────────────────────────────────

func TestGetStorageUsage_HappyPath(t *testing.T) {
	f := newFixture()
	f.blobs.storageUsed = 500
	f.svc.QuotaBytes = 1000

	usage, err := f.svc.GetStorageUsage(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetStorageUsage: %v", err)
	}
	if usage.UsedBytes != 500 {
		t.Errorf("UsedBytes: want 500, got %d", usage.UsedBytes)
	}
	if usage.QuotaBytes != 1000 {
		t.Errorf("QuotaBytes: want 1000, got %d", usage.QuotaBytes)
	}
}

func TestGetStorageUsage_RepoError(t *testing.T) {
	f := newFixture()
	f.blobs.storageUsedErr = errors.New("db error")

	_, err := f.svc.GetStorageUsage(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── PresignPut ───────────────────────────────────────────────────────────────

func TestPresignPut_HappyPath(t *testing.T) {
	f := newFixture()

	result, err := f.svc.PresignPut(context.Background(), storage.PresignPutParams{
		UserID:   uuid.New(),
		FileName: "report.pdf",
		FileSize: 100,
	})
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	if result.BlobID == uuid.Nil {
		t.Error("expected non-nil BlobID")
	}
	if result.UploadURL == "" {
		t.Error("expected non-empty UploadURL")
	}
	if result.HTTPMethod != "PUT" {
		t.Errorf("HTTPMethod: want PUT, got %s", result.HTTPMethod)
	}
}

func TestPresignPut_FileTooLarge(t *testing.T) {
	f := newFixture()
	f.svc.MaxUploadBytes = 1024

	_, err := f.svc.PresignPut(context.Background(), storage.PresignPutParams{
		UserID:   uuid.New(),
		FileSize: 2048,
	})
	if !errors.Is(err, storage.ErrFileTooLarge) {
		t.Fatalf("want ErrFileTooLarge, got %v", err)
	}
}

func TestPresignPut_FileSizeAtLimit_Allowed(t *testing.T) {
	f := newFixture()
	f.svc.MaxUploadBytes = 1024

	_, err := f.svc.PresignPut(context.Background(), storage.PresignPutParams{
		UserID:   uuid.New(),
		FileSize: 1024, // exactly at limit — must pass
	})
	if errors.Is(err, storage.ErrFileTooLarge) {
		t.Fatal("file at exact limit should be allowed")
	}
}

func TestPresignPut_QuotaExceeded(t *testing.T) {
	f := newFixture()
	f.svc.QuotaBytes = 1000
	f.blobs.storageUsed = 900

	_, err := f.svc.PresignPut(context.Background(), storage.PresignPutParams{
		UserID:   uuid.New(),
		FileSize: 200, // 900 + 200 = 1100 > 1000
	})
	if !errors.Is(err, storage.ErrQuotaExceeded) {
		t.Fatalf("want ErrQuotaExceeded, got %v", err)
	}
}

func TestPresignPut_QuotaExactBoundary_Allowed(t *testing.T) {
	f := newFixture()
	f.svc.QuotaBytes = 1000
	f.blobs.storageUsed = 800

	_, err := f.svc.PresignPut(context.Background(), storage.PresignPutParams{
		UserID:   uuid.New(),
		FileSize: 200, // 800 + 200 == 1000 — not strictly greater, must pass
	})
	if errors.Is(err, storage.ErrQuotaExceeded) {
		t.Fatal("used + size == quota should be allowed")
	}
}

func TestPresignPut_NoQuotaCheck_WhenQuotaZero(t *testing.T) {
	f := newFixture()
	// QuotaBytes = 0 means unlimited — storageUsedErr must never be hit.
	f.blobs.storageUsedErr = errors.New("should not be called")

	_, err := f.svc.PresignPut(context.Background(), storage.PresignPutParams{
		UserID:   uuid.New(),
		FileSize: 999_999_999,
	})
	if errors.Is(err, storage.ErrQuotaExceeded) {
		t.Fatal("quota must not be checked when QuotaBytes is zero")
	}
	// any error here would be from storageUsedErr being hit
	if err != nil && err.Error() == "get storage used: should not be called" {
		t.Fatal("GetUserStorageUsed must not be called when QuotaBytes is zero")
	}
}

func TestPresignPut_FolderNotFound(t *testing.T) {
	f := newFixture()
	f.folders.folderOk = false

	folderID := uuid.New()
	_, err := f.svc.PresignPut(context.Background(), storage.PresignPutParams{
		UserID:   uuid.New(),
		FileSize: 100,
		FolderID: &folderID,
	})
	if !errors.Is(err, storage.ErrFolderNotFound) {
		t.Fatalf("want ErrFolderNotFound, got %v", err)
	}
}

func TestPresignPut_RegisterBlobError(t *testing.T) {
	f := newFixture()
	f.blobs.registerErr = errors.New("insert failed")

	_, err := f.svc.PresignPut(context.Background(), storage.PresignPutParams{
		UserID:   uuid.New(),
		FileSize: 100,
	})
	if err == nil {
		t.Fatal("expected error from RegisterBlob")
	}
}

func TestPresignPut_PresignObjectError(t *testing.T) {
	f := newFixture()
	f.objects.presignPutErr = errors.New("s3 unavailable")

	_, err := f.svc.PresignPut(context.Background(), storage.PresignPutParams{
		UserID:   uuid.New(),
		FileSize: 100,
	})
	if err == nil {
		t.Fatal("expected error from PresignedPutObject")
	}
}

// ─── ConfirmUpload ────────────────────────────────────────────────────────────

func TestConfirmUpload_HappyPath(t *testing.T) {
	f := newFixture()
	f.blobs.pending = storage.PendingBlobMeta{
		ObjectKey:    "user/blob1",
		DeclaredSize: 500,
		FileName:     "photo.jpg",
	}
	f.blobs.pendingFound = true
	f.objects.statSize = 500 // actual == declared
	f.blobs.confirmOk = true

	fileName, folderName, err := f.svc.ConfirmUpload(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("ConfirmUpload: %v", err)
	}
	if fileName != "photo.jpg" {
		t.Errorf("fileName: want photo.jpg, got %s", fileName)
	}
	if folderName != "" {
		t.Errorf("folderName: want empty for root, got %s", folderName)
	}
}

func TestConfirmUpload_BlobNotFound(t *testing.T) {
	f := newFixture()
	f.blobs.pendingFound = false

	_, _, err := f.svc.ConfirmUpload(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestConfirmUpload_StatObjectError(t *testing.T) {
	f := newFixture()
	f.blobs.pendingFound = true
	f.blobs.pending = storage.PendingBlobMeta{ObjectKey: "user/blob"}
	f.objects.statErr = errors.New("s3 unreachable")

	_, _, err := f.svc.ConfirmUpload(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error from StatObject")
	}
}

func TestConfirmUpload_SizeMismatch_QuotaExceeded_DeletesObject(t *testing.T) {
	f := newFixture()
	objectKey := "user/blob-big"
	f.blobs.pendingFound = true
	f.blobs.pending = storage.PendingBlobMeta{
		ObjectKey:    objectKey,
		DeclaredSize: 100,
	}
	f.objects.statSize = 900  // actual > declared
	f.svc.QuotaBytes = 1000   // quota is 1000
	f.blobs.storageUsed = 900 // used (includes declared 100) → actual pushes to 1700

	// used - declared + actual = 900 - 100 + 900 = 1700 > 1000 → quota exceeded
	_, _, err := f.svc.ConfirmUpload(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrQuotaExceeded) {
		t.Fatalf("want ErrQuotaExceeded, got %v", err)
	}

	// object must be removed to free space
	if len(f.objects.removedKeys) == 0 || f.objects.removedKeys[0] != objectKey {
		t.Error("object must be deleted from S3 when quota is exceeded on confirm")
	}
}

func TestConfirmUpload_SizeMismatch_WithinQuota_Passes(t *testing.T) {
	f := newFixture()
	f.blobs.pendingFound = true
	f.blobs.pending = storage.PendingBlobMeta{
		ObjectKey:    "user/blob",
		DeclaredSize: 100,
	}
	f.objects.statSize = 200 // actual > declared, but still within quota
	f.svc.QuotaBytes = 10_000
	f.blobs.storageUsed = 300 // 300 - 100 + 200 = 400 < 10000 → ok
	f.blobs.confirmOk = true

	_, _, err := f.svc.ConfirmUpload(context.Background(), uuid.New(), uuid.New())
	if errors.Is(err, storage.ErrQuotaExceeded) {
		t.Fatal("should pass when adjusted usage is within quota")
	}
}

func TestConfirmUpload_ConfirmReturnsFalse(t *testing.T) {
	f := newFixture()
	f.blobs.pendingFound = true
	f.blobs.pending = storage.PendingBlobMeta{ObjectKey: "k", DeclaredSize: 10}
	f.objects.statSize = 10
	f.blobs.confirmOk = false // race: another request confirmed first

	_, _, err := f.svc.ConfirmUpload(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("want ErrNotFound when confirm returns false, got %v", err)
	}
}

func TestConfirmUpload_WithFolder_EnrichersFolderName(t *testing.T) {
	f := newFixture()
	folderID := uuid.New()
	f.blobs.pendingFound = true
	f.blobs.pending = storage.PendingBlobMeta{
		ObjectKey:    "u/b",
		DeclaredSize: 10,
		FileName:     "doc.pdf",
		FolderID:     &folderID,
	}
	f.objects.statSize = 10
	f.blobs.confirmOk = true
	f.folders.folder = entity.Folder{ID: folderID, Name: "Documents"}
	f.folders.folderOk = true

	_, folderName, err := f.svc.ConfirmUpload(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("ConfirmUpload: %v", err)
	}
	if folderName != "Documents" {
		t.Errorf("folderName: want Documents, got %q", folderName)
	}
}

// ─── DeleteBlob (trash) ───────────────────────────────────────────────────────

func TestDeleteBlob_HappyPath(t *testing.T) {
	f := newFixture()
	f.blobs.trashName = "file.pdf"
	f.blobs.trashOk = true

	name, err := f.svc.DeleteBlob(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("DeleteBlob: %v", err)
	}
	if name != "file.pdf" {
		t.Errorf("name: want file.pdf, got %s", name)
	}
}

func TestDeleteBlob_NotFound(t *testing.T) {
	f := newFixture()
	f.blobs.trashOk = false

	_, err := f.svc.DeleteBlob(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// ─── HardDeleteBlob ───────────────────────────────────────────────────────────

func TestHardDeleteBlob_HappyPath_RemovesObject(t *testing.T) {
	f := newFixture()
	f.blobTrash.hardDeleteKey = "user/blob123"
	f.blobTrash.hardDeleteName = "archive.zip"
	f.blobTrash.hardDeleteOk = true

	name, err := f.svc.HardDeleteBlob(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("HardDeleteBlob: %v", err)
	}
	if name != "archive.zip" {
		t.Errorf("name: want archive.zip, got %s", name)
	}
	if len(f.objects.removedKeys) == 0 || f.objects.removedKeys[0] != "user/blob123" {
		t.Error("expected RemoveObject called with the blob's object key")
	}
}

func TestHardDeleteBlob_NotFound(t *testing.T) {
	f := newFixture()
	f.blobTrash.hardDeleteOk = false

	_, err := f.svc.HardDeleteBlob(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestHardDeleteBlob_RemoveObjectError(t *testing.T) {
	f := newFixture()
	f.blobTrash.hardDeleteKey = "user/blob"
	f.blobTrash.hardDeleteOk = true
	f.objects.removeErr = errors.New("s3 error")

	_, err := f.svc.HardDeleteBlob(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error when RemoveObject fails")
	}
}

// ─── MoveFolder ───────────────────────────────────────────────────────────────

func TestMoveFolder_SourceNotFound(t *testing.T) {
	f := newFixture()
	f.folders.folderOk = false

	_, err := f.svc.MoveFolder(context.Background(), storage.MoveFolderParams{
		FolderID: uuid.New(),
		UserID:   uuid.New(),
	})
	if !errors.Is(err, storage.ErrFolderNotFound) {
		t.Fatalf("want ErrFolderNotFound, got %v", err)
	}
}

func TestMoveFolder_SelfCycle(t *testing.T) {
	f := newFixture()
	folderID := uuid.New()
	f.folders.folder = entity.Folder{ID: folderID, Name: "Docs"}
	f.folders.folderOk = true

	_, err := f.svc.MoveFolder(context.Background(), storage.MoveFolderParams{
		FolderID:    folderID,
		UserID:      uuid.New(),
		NewParentID: &folderID, // move into itself
	})
	if !errors.Is(err, storage.ErrFolderCycle) {
		t.Fatalf("want ErrFolderCycle for self-move, got %v", err)
	}
}

func TestMoveFolder_DestinationNotFound(t *testing.T) {
	f := newFixture()
	folderID := uuid.New()
	dstID := uuid.New()

	// First call (source) succeeds; second call (destination) returns not found.
	callCount := 0
	f.folders.getFolderFn = func(_ context.Context, id, _ uuid.UUID) (entity.Folder, bool, error) {
		callCount++
		if id == folderID {
			return entity.Folder{ID: folderID, Name: "Docs"}, true, nil
		}
		return entity.Folder{}, false, nil // destination not found
	}

	_, err := f.svc.MoveFolder(context.Background(), storage.MoveFolderParams{
		FolderID:    folderID,
		UserID:      uuid.New(),
		NewParentID: &dstID,
	})
	if !errors.Is(err, storage.ErrFolderNotFound) {
		t.Fatalf("want ErrFolderNotFound for missing destination, got %v", err)
	}
}

func TestMoveFolder_DescendantCycle(t *testing.T) {
	f := newFixture()
	folderID := uuid.New()
	dstID := uuid.New()

	f.folders.getFolderFn = func(_ context.Context, id, _ uuid.UUID) (entity.Folder, bool, error) {
		if id == folderID {
			return entity.Folder{ID: folderID, Name: "Parent"}, true, nil
		}
		return entity.Folder{ID: dstID, Name: "Child"}, true, nil
	}
	f.folders.isDescendant = true // dstID is a descendant of folderID

	_, err := f.svc.MoveFolder(context.Background(), storage.MoveFolderParams{
		FolderID:    folderID,
		UserID:      uuid.New(),
		NewParentID: &dstID,
	})
	if !errors.Is(err, storage.ErrFolderCycle) {
		t.Fatalf("want ErrFolderCycle for descendant move, got %v", err)
	}
}

func TestMoveFolder_ToRoot_HappyPath(t *testing.T) {
	f := newFixture()
	folderID := uuid.New()
	f.folders.folder = entity.Folder{ID: folderID, Name: "Docs"}
	f.folders.folderOk = true

	info, err := f.svc.MoveFolder(context.Background(), storage.MoveFolderParams{
		FolderID:    folderID,
		UserID:      uuid.New(),
		NewParentID: nil, // move to root
	})
	if err != nil {
		t.Fatalf("MoveFolder to root: %v", err)
	}
	if info.FolderName != "Docs" {
		t.Errorf("FolderName: want Docs, got %s", info.FolderName)
	}
	if info.DstFolderName != "" {
		t.Errorf("DstFolderName: want empty for root, got %s", info.DstFolderName)
	}
}

func TestMoveFolder_ToFolder_HappyPath(t *testing.T) {
	f := newFixture()
	folderID := uuid.New()
	dstID := uuid.New()

	f.folders.getFolderFn = func(_ context.Context, id, _ uuid.UUID) (entity.Folder, bool, error) {
		if id == folderID {
			return entity.Folder{ID: folderID, Name: "Docs"}, true, nil
		}
		return entity.Folder{ID: dstID, Name: "Archive"}, true, nil
	}

	info, err := f.svc.MoveFolder(context.Background(), storage.MoveFolderParams{
		FolderID:    folderID,
		UserID:      uuid.New(),
		NewParentID: &dstID,
	})
	if err != nil {
		t.Fatalf("MoveFolder to folder: %v", err)
	}
	if info.DstFolderName != "Archive" {
		t.Errorf("DstFolderName: want Archive, got %s", info.DstFolderName)
	}
}

// ─── EmptyTrash ───────────────────────────────────────────────────────────────

func TestEmptyTrash_HappyPath_RemovesAllObjects(t *testing.T) {
	f := newFixture()
	f.blobTrash.emptiedBlobs = []storage.EmptiedBlob{
		{ObjectKey: "u/blob1"},
		{ObjectKey: "u/blob2"},
	}
	f.folderTrash.emptiedKeys = []string{"u/folder-blob1", "u/folder-blob2"}

	_, _, err := f.svc.EmptyTrash(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("EmptyTrash: %v", err)
	}

	want := map[string]bool{
		"u/blob1": true, "u/blob2": true,
		"u/folder-blob1": true, "u/folder-blob2": true,
	}
	if len(f.objects.removedKeys) != 4 {
		t.Fatalf("want 4 RemoveObject calls, got %d", len(f.objects.removedKeys))
	}
	for _, key := range f.objects.removedKeys {
		if !want[key] {
			t.Errorf("unexpected removed key: %s", key)
		}
	}
}

func TestEmptyTrash_BlobTrashError(t *testing.T) {
	f := newFixture()
	f.blobTrash.emptyErr = errors.New("db error")

	_, _, err := f.svc.EmptyTrash(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error from EmptyTrashedBlobs")
	}
}

func TestEmptyTrash_FolderTrashError(t *testing.T) {
	f := newFixture()
	f.folderTrash.emptyErr = errors.New("db error")

	_, _, err := f.svc.EmptyTrash(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error from EmptyTrashedFolders")
	}
}

// ─── ListTrash ────────────────────────────────────────────────────────────────

func TestListTrash_HappyPath(t *testing.T) {
	f := newFixture()
	f.blobTrash.listedBlobs = []entity.Blob{{ID: uuid.New(), FileName: "file.pdf"}}
	f.folderTrash.listedFolders = []entity.Folder{{ID: uuid.New(), Name: "Old docs"}}

	result, err := f.svc.ListTrash(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("ListTrash: %v", err)
	}
	if len(result.Blobs) != 1 {
		t.Errorf("Blobs: want 1, got %d", len(result.Blobs))
	}
	if len(result.Folders) != 1 {
		t.Errorf("Folders: want 1, got %d", len(result.Folders))
	}
}

func TestListTrash_BlobError_Propagates(t *testing.T) {
	f := newFixture()
	f.blobTrash.listErr = errors.New("db error")

	_, err := f.svc.ListTrash(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error from ListTrashedBlobs")
	}
}

func TestListTrash_FolderError_Propagates(t *testing.T) {
	f := newFixture()
	f.folderTrash.listErr = errors.New("db error")

	_, err := f.svc.ListTrash(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error from ListTrashedFolders")
	}
}
