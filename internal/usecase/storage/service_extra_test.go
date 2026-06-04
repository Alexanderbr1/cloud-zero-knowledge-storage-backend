package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
	storage "cloud-backend/internal/usecase/storage"
)

var ctx = context.Background()

// ─── PresignGet ───────────────────────────────────────────────────────────────

func TestPresignGet_HappyPath(t *testing.T) {
	f := newFixture()
	blobID := uuid.New()
	userID := uuid.New()

	f.blobs.blobMeta = storage.BlobMeta{
		ObjectKey:        "obj/key",
		ContentType:      "image/jpeg",
		FileName:         "photo.jpg",
		EncryptedFileKey: []byte("efk=="),
		ChunkSize:        4194304, FileSizePlain: 100,
	}
	f.blobs.blobMetaFound = true

	res, err := f.svc.PresignGet(ctx, userID, blobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.DownloadURL == "" {
		t.Error("expected non-empty DownloadURL")
	}
	if res.ContentType != "image/jpeg" {
		t.Errorf("content type: got %q", res.ContentType)
	}
	if string(res.EncryptedFileKey) != "efk==" {
		t.Errorf("encrypted file key: got %q", res.EncryptedFileKey)
	}
	if res.HTTPMethod != "GET" {
		t.Errorf("http method: got %q", res.HTTPMethod)
	}
}

func TestPresignGet_NotFound_ReturnsErrNotFound(t *testing.T) {
	f := newFixture()
	f.blobs.blobMetaFound = false

	_, err := f.svc.PresignGet(ctx, uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestPresignGet_RepoError_Propagates(t *testing.T) {
	f := newFixture()
	f.blobs.blobMetaErr = errors.New("db down")

	_, err := f.svc.PresignGet(ctx, uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPresignGet_PresignError_Propagates(t *testing.T) {
	f := newFixture()
	f.blobs.blobMetaFound = true
	f.objects.presignGetErr = errors.New("s3 unavailable")

	_, err := f.svc.PresignGet(ctx, uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── RestoreBlob ──────────────────────────────────────────────────────────────

func TestRestoreBlob_HappyPath(t *testing.T) {
	f := newFixture()
	f.blobTrash.restoreName = "document.pdf"
	f.blobTrash.restoreOk = true

	name, err := f.svc.RestoreBlob(ctx, uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "document.pdf" {
		t.Errorf("name: got %q, want %q", name, "document.pdf")
	}
}

func TestRestoreBlob_NotFound_ReturnsErrNotFound(t *testing.T) {
	f := newFixture()
	f.blobTrash.restoreOk = false

	_, err := f.svc.RestoreBlob(ctx, uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRestoreBlob_RepoError_Propagates(t *testing.T) {
	f := newFixture()
	f.blobTrash.restoreErr = errors.New("db error")

	_, err := f.svc.RestoreBlob(ctx, uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── RestoreFolder ────────────────────────────────────────────────────────────

func TestRestoreFolder_HappyPath(t *testing.T) {
	f := newFixture()
	f.folderTrash.restoreName = "Archive"
	f.folderTrash.restoreOk = true

	name, err := f.svc.RestoreFolder(ctx, uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "Archive" {
		t.Errorf("name: got %q, want %q", name, "Archive")
	}
}

func TestRestoreFolder_NotFound_ReturnsErrFolderNotFound(t *testing.T) {
	f := newFixture()
	f.folderTrash.restoreOk = false

	_, err := f.svc.RestoreFolder(ctx, uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrFolderNotFound) {
		t.Errorf("expected ErrFolderNotFound, got %v", err)
	}
}

func TestRestoreFolder_RepoError_Propagates(t *testing.T) {
	f := newFixture()
	f.folderTrash.restoreErr = errors.New("db error")

	_, err := f.svc.RestoreFolder(ctx, uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── HardDeleteFolder ─────────────────────────────────────────────────────────

func TestHardDeleteFolder_HappyPath_RemovesAllObjectKeys(t *testing.T) {
	f := newFixture()
	f.folderTrash.hardDeleteKeys = []string{"obj/a", "obj/b", "obj/c"}
	f.folderTrash.hardDeleteName = "Photos"
	f.folderTrash.hardDeleteOk = true

	name, err := f.svc.HardDeleteFolder(ctx, uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "Photos" {
		t.Errorf("name: got %q", name)
	}
	f.objects.mu.Lock()
	removed := f.objects.removedKeys
	f.objects.mu.Unlock()
	if len(removed) != 3 {
		t.Errorf("expected 3 object removals, got %d", len(removed))
	}
}

func TestHardDeleteFolder_NotFound_ReturnsErrFolderNotFound(t *testing.T) {
	f := newFixture()
	f.folderTrash.hardDeleteOk = false

	_, err := f.svc.HardDeleteFolder(ctx, uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrFolderNotFound) {
		t.Errorf("expected ErrFolderNotFound, got %v", err)
	}
}

func TestHardDeleteFolder_ObjectRemoveErrors_AreIgnored(t *testing.T) {
	// HardDeleteFolder uses best-effort removal — object errors are swallowed.
	f := newFixture()
	f.folderTrash.hardDeleteKeys = []string{"obj/x"}
	f.folderTrash.hardDeleteName = "Docs"
	f.folderTrash.hardDeleteOk = true
	f.objects.removeErr = errors.New("s3 error")

	name, err := f.svc.HardDeleteFolder(ctx, uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "Docs" {
		t.Errorf("name: got %q", name)
	}
}

// ─── MoveBlob ─────────────────────────────────────────────────────────────────

func TestMoveBlob_HappyPath_ToRoot(t *testing.T) {
	f := newFixture()
	f.blobs.moveInfo = storage.MoveBlobInfo{FileName: "report.pdf"}
	f.blobs.moveOk = true

	info, err := f.svc.MoveBlob(ctx, uuid.New(), uuid.New(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.FileName != "report.pdf" {
		t.Errorf("filename: got %q", info.FileName)
	}
	if info.DstFolderName != "" {
		t.Errorf("dst folder name should be empty for root move, got %q", info.DstFolderName)
	}
}

func TestMoveBlob_HappyPath_ToFolder_EnrichesDstFolderName(t *testing.T) {
	f := newFixture()
	folderID := uuid.New()
	f.folders.folder = entity.Folder{ID: folderID, Name: "Projects"}
	f.folders.folderOk = true
	f.blobs.moveInfo = storage.MoveBlobInfo{FileName: "spec.docx"}
	f.blobs.moveOk = true

	info, err := f.svc.MoveBlob(ctx, uuid.New(), uuid.New(), &folderID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.DstFolderName != "Projects" {
		t.Errorf("dst folder name: got %q, want %q", info.DstFolderName, "Projects")
	}
}

func TestMoveBlob_FolderNotFound_ReturnsErrFolderNotFound(t *testing.T) {
	f := newFixture()
	folderID := uuid.New()
	f.folders.folderOk = false

	_, err := f.svc.MoveBlob(ctx, uuid.New(), uuid.New(), &folderID)
	if !errors.Is(err, storage.ErrFolderNotFound) {
		t.Errorf("expected ErrFolderNotFound, got %v", err)
	}
}

func TestMoveBlob_BlobNotFound_ReturnsErrNotFound(t *testing.T) {
	f := newFixture()
	f.blobs.moveOk = false

	_, err := f.svc.MoveBlob(ctx, uuid.New(), uuid.New(), nil)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ─── RenameBlob ───────────────────────────────────────────────────────────────

func TestRenameBlob_HappyPath(t *testing.T) {
	f := newFixture()
	f.blobs.renameOk = true

	if err := f.svc.RenameBlob(ctx, uuid.New(), uuid.New(), "new name.pdf"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenameBlob_NotFound_ReturnsErrNotFound(t *testing.T) {
	f := newFixture()
	f.blobs.renameOk = false

	err := f.svc.RenameBlob(ctx, uuid.New(), uuid.New(), "name.pdf")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRenameBlob_NameIsSanitized(t *testing.T) {
	// The name "  ../secret.txt  " should be sanitized to "secret.txt"
	// before being stored. We verify no error is returned (mock renameOk=true)
	// and the repo is called (renameOk means it was reached).
	f := newFixture()
	f.blobs.renameOk = true

	if err := f.svc.RenameBlob(ctx, uuid.New(), uuid.New(), "  ../secret.txt  "); err != nil {
		t.Fatalf("unexpected error after sanitization: %v", err)
	}
}

// ─── CreateFolder ─────────────────────────────────────────────────────────────

func TestCreateFolder_AtRoot_HappyPath(t *testing.T) {
	f := newFixture()
	folderID := uuid.New()
	f.folders.createFolder = entity.Folder{ID: folderID, Name: "Docs"}

	folder, parentName, err := f.svc.CreateFolder(ctx, storage.CreateFolderParams{
		Name:   "Docs",
		UserID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder.Name != "Docs" {
		t.Errorf("name: got %q", folder.Name)
	}
	if parentName != "" {
		t.Errorf("parentName should be empty for root folder, got %q", parentName)
	}
}

func TestCreateFolder_WithParent_EnrichesParentName(t *testing.T) {
	f := newFixture()
	parentID := uuid.New()
	f.folders.folder = entity.Folder{ID: parentID, Name: "Projects"}
	f.folders.folderOk = true
	f.folders.createFolder = entity.Folder{Name: "2025"}

	p := storage.CreateFolderParams{
		Name:     "2025",
		UserID:   uuid.New(),
		ParentID: &parentID,
	}
	_, parentName, err := f.svc.CreateFolder(ctx, p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parentName != "Projects" {
		t.Errorf("parentName: got %q, want %q", parentName, "Projects")
	}
}

func TestCreateFolder_ParentNotFound_ReturnsErrFolderNotFound(t *testing.T) {
	f := newFixture()
	parentID := uuid.New()
	f.folders.folderOk = false

	_, _, err := f.svc.CreateFolder(ctx, storage.CreateFolderParams{
		Name:     "Sub",
		UserID:   uuid.New(),
		ParentID: &parentID,
	})
	if !errors.Is(err, storage.ErrFolderNotFound) {
		t.Errorf("expected ErrFolderNotFound, got %v", err)
	}
}

// ─── GetFolder ────────────────────────────────────────────────────────────────

func TestGetFolder_HappyPath(t *testing.T) {
	f := newFixture()
	folderID := uuid.New()
	f.folders.folder = entity.Folder{ID: folderID, Name: "Reports"}
	f.folders.folderOk = true

	folder, err := f.svc.GetFolder(ctx, uuid.New(), folderID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder.Name != "Reports" {
		t.Errorf("name: got %q", folder.Name)
	}
}

func TestGetFolder_NotFound_ReturnsErrFolderNotFound(t *testing.T) {
	f := newFixture()
	f.folders.folderOk = false

	_, err := f.svc.GetFolder(ctx, uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrFolderNotFound) {
		t.Errorf("expected ErrFolderNotFound, got %v", err)
	}
}

// ─── ListBlobsInFolder ────────────────────────────────────────────────────────

func TestListBlobsInFolder_NilFolderID_SkipsFolderCheck(t *testing.T) {
	f := newFixture()
	f.blobs.listInFolderResult = []entity.Blob{{ID: uuid.New(), FileName: "root.pdf"}}

	blobs, err := f.svc.ListBlobsInFolder(ctx, uuid.New(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blobs) != 1 {
		t.Errorf("expected 1 blob, got %d", len(blobs))
	}
}

func TestListBlobsInFolder_FolderNotFound_ReturnsErrFolderNotFound(t *testing.T) {
	f := newFixture()
	folderID := uuid.New()
	f.folders.folderOk = false // requireFolder fails

	_, err := f.svc.ListBlobsInFolder(ctx, uuid.New(), &folderID)
	if !errors.Is(err, storage.ErrFolderNotFound) {
		t.Errorf("expected ErrFolderNotFound, got %v", err)
	}
}

// ─── RenameFolder ─────────────────────────────────────────────────────────────

func TestRenameFolder_HappyPath(t *testing.T) {
	f := newFixture()
	f.folders.renameFolderResult = entity.Folder{Name: "Renamed"}

	folder, err := f.svc.RenameFolder(ctx, uuid.New(), uuid.New(), "Renamed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if folder.Name != "Renamed" {
		t.Errorf("name: got %q", folder.Name)
	}
}

func TestRenameFolder_RepoError_Propagates(t *testing.T) {
	f := newFixture()
	f.folders.renameFolderErr = errors.New("conflict")

	_, err := f.svc.RenameFolder(ctx, uuid.New(), uuid.New(), "Taken")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ─── DeleteFolder ─────────────────────────────────────────────────────────────

func TestDeleteFolder_HappyPath(t *testing.T) {
	f := newFixture()
	f.folders.folder = entity.Folder{Name: "Old Docs"}
	f.folders.folderOk = true
	f.folders.trashOk = true

	name, err := f.svc.DeleteFolder(ctx, uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "Old Docs" {
		t.Errorf("name: got %q", name)
	}
}

func TestDeleteFolder_FolderNotFound_DuringGet(t *testing.T) {
	f := newFixture()
	f.folders.folderOk = false

	_, err := f.svc.DeleteFolder(ctx, uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrFolderNotFound) {
		t.Errorf("expected ErrFolderNotFound, got %v", err)
	}
}

func TestDeleteFolder_TrashReturnsFalse_ReturnsErrFolderNotFound(t *testing.T) {
	f := newFixture()
	f.folders.folder = entity.Folder{Name: "Docs"}
	f.folders.folderOk = true
	f.folders.trashOk = false // TrashFolder returns (false, nil)

	_, err := f.svc.DeleteFolder(ctx, uuid.New(), uuid.New())
	if !errors.Is(err, storage.ErrFolderNotFound) {
		t.Errorf("expected ErrFolderNotFound, got %v", err)
	}
}

// ─── Search ───────────────────────────────────────────────────────────────────

func TestSearch_HappyPath_RunsInParallel(t *testing.T) {
	f := newFixture()
	f.blobs.searchBlobsResult = []storage.SearchBlobRecord{{Blob: entity.Blob{FileName: "report.pdf"}}}
	f.folders.searchFoldersResult = []entity.Folder{{Name: "Archive"}}

	res, err := f.svc.Search(ctx, storage.SearchParams{
		UserID: uuid.New(),
		Query:  "archive",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Blobs) != 1 {
		t.Errorf("blobs: got %d, want 1", len(res.Blobs))
	}
	if len(res.Folders) != 1 {
		t.Errorf("folders: got %d, want 1", len(res.Folders))
	}
}

func TestSearch_BlobError_Propagates(t *testing.T) {
	f := newFixture()
	f.blobs.searchBlobsErr = errors.New("search index unavailable")

	_, err := f.svc.Search(ctx, storage.SearchParams{UserID: uuid.New(), Query: "q"})
	if err == nil {
		t.Fatal("expected error from blob search")
	}
}

func TestSearch_FolderError_Propagates(t *testing.T) {
	f := newFixture()
	f.folders.searchFoldersErr = errors.New("search index unavailable")

	_, err := f.svc.Search(ctx, storage.SearchParams{UserID: uuid.New(), Query: "q"})
	if err == nil {
		t.Fatal("expected error from folder search")
	}
}

// ─── CleanOrphanedBlobs ───────────────────────────────────────────────────────

func TestCleanOrphanedBlobs_HappyPath_RemovesObjectKeys(t *testing.T) {
	f := newFixture()
	f.blobs.purgeOrphanedKeys = []storage.OrphanedBlob{{ObjectKey: "obj/orphan1"}, {ObjectKey: "obj/orphan2"}}

	if err := f.svc.CleanOrphanedBlobs(ctx, 24*time.Hour); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f.objects.mu.Lock()
	removed := f.objects.removedKeys
	f.objects.mu.Unlock()

	if len(removed) != 2 {
		t.Errorf("expected 2 removals, got %d", len(removed))
	}
}

func TestCleanOrphanedBlobs_NoOrphans_NoObjectCalls(t *testing.T) {
	f := newFixture()
	f.blobs.purgeOrphanedKeys = nil

	if err := f.svc.CleanOrphanedBlobs(ctx, time.Hour); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f.objects.mu.Lock()
	removed := f.objects.removedKeys
	f.objects.mu.Unlock()
	if len(removed) != 0 {
		t.Errorf("expected no removals, got %d", len(removed))
	}
}

func TestCleanOrphanedBlobs_RepoError_Propagates(t *testing.T) {
	f := newFixture()
	f.blobs.purgeOrphanedErr = errors.New("db error")

	if err := f.svc.CleanOrphanedBlobs(ctx, time.Hour); err == nil {
		t.Fatal("expected error")
	}
}

// ─── CleanExpiredTrash ────────────────────────────────────────────────────────

func TestCleanExpiredTrash_HappyPath_RemovesBlobAndFolderKeys(t *testing.T) {
	f := newFixture()
	f.blobTrash.purgeExpiredKeys = []string{"blob/old1", "blob/old2"}
	f.folderTrash.purgeExpiredKeys = []string{"folder/file1", "folder/file2", "folder/file3"}

	if err := f.svc.CleanExpiredTrash(ctx, 7*24*time.Hour); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f.objects.mu.Lock()
	removed := f.objects.removedKeys
	f.objects.mu.Unlock()

	// 2 blob keys + 3 folder blob keys = 5 total removals
	if len(removed) != 5 {
		t.Errorf("expected 5 removals, got %d: %v", len(removed), removed)
	}
}

func TestCleanExpiredTrash_BlobPurgeError_Propagates(t *testing.T) {
	f := newFixture()
	f.blobTrash.purgeExpiredErr = errors.New("db error")

	if err := f.svc.CleanExpiredTrash(ctx, time.Hour); err == nil {
		t.Fatal("expected error from blob purge")
	}
}

func TestCleanExpiredTrash_FolderPurgeError_Propagates(t *testing.T) {
	f := newFixture()
	f.folderTrash.purgeExpiredErr = errors.New("db error")

	if err := f.svc.CleanExpiredTrash(ctx, time.Hour); err == nil {
		t.Fatal("expected error from folder purge")
	}
}

func TestCleanExpiredTrash_ObjectRemoveErrors_AreIgnored(t *testing.T) {
	// best-effort: object store errors must not abort the job
	f := newFixture()
	f.blobTrash.purgeExpiredKeys = []string{"k1"}
	f.folderTrash.purgeExpiredKeys = []string{"k2"}
	f.objects.removeErr = errors.New("s3 error")

	if err := f.svc.CleanExpiredTrash(ctx, time.Hour); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
