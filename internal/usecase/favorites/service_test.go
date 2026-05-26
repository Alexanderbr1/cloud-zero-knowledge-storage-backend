package favorites_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
	"cloud-backend/internal/usecase/favorites"
)

// ─── mock repo ────────────────────────────────────────────────────────────────

type mockFavRepo struct {
	addBlobErr        error
	removeBlobErr     error
	addFolderErr      error
	removeFolderErr   error
	listBlobsResult   []favorites.FavoriteBlob
	listBlobsErr      error
	listFoldersResult []entity.Folder
	listFoldersErr    error
}

func (m *mockFavRepo) AddBlobFavorite(_ context.Context, _, _ uuid.UUID) error {
	return m.addBlobErr
}
func (m *mockFavRepo) RemoveBlobFavorite(_ context.Context, _, _ uuid.UUID) error {
	return m.removeBlobErr
}
func (m *mockFavRepo) AddFolderFavorite(_ context.Context, _, _ uuid.UUID) error {
	return m.addFolderErr
}
func (m *mockFavRepo) RemoveFolderFavorite(_ context.Context, _, _ uuid.UUID) error {
	return m.removeFolderErr
}
func (m *mockFavRepo) ListFavoriteBlobs(_ context.Context, _ uuid.UUID) ([]favorites.FavoriteBlob, error) {
	return m.listBlobsResult, m.listBlobsErr
}
func (m *mockFavRepo) ListFavoriteFolders(_ context.Context, _ uuid.UUID) ([]entity.Folder, error) {
	return m.listFoldersResult, m.listFoldersErr
}

func newFavSvc(repo *mockFavRepo) *favorites.Service {
	return &favorites.Service{Repo: repo}
}

var bgCtx = context.Background()

// ─── AddBlob ─────────────────────────────────────────────────────────────────

func TestAddBlob_HappyPath(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{})
	if err := svc.AddBlob(bgCtx, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddBlob_RepoError_Propagates(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{addBlobErr: errors.New("duplicate key")})
	if err := svc.AddBlob(bgCtx, uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected error")
	}
}

// ─── RemoveBlob ───────────────────────────────────────────────────────────────

func TestRemoveBlob_HappyPath(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{})
	if err := svc.RemoveBlob(bgCtx, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveBlob_RepoError_Propagates(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{removeBlobErr: errors.New("not found")})
	if err := svc.RemoveBlob(bgCtx, uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected error")
	}
}

// ─── AddFolder ────────────────────────────────────────────────────────────────

func TestAddFolder_HappyPath(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{})
	if err := svc.AddFolder(bgCtx, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddFolder_RepoError_Propagates(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{addFolderErr: errors.New("duplicate key")})
	if err := svc.AddFolder(bgCtx, uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected error")
	}
}

// ─── RemoveFolder ─────────────────────────────────────────────────────────────

func TestRemoveFolder_HappyPath(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{})
	if err := svc.RemoveFolder(bgCtx, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveFolder_RepoError_Propagates(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{removeFolderErr: errors.New("not found")})
	if err := svc.RemoveFolder(bgCtx, uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected error")
	}
}

// ─── ListBlobs ────────────────────────────────────────────────────────────────

func TestListBlobs_ReturnsRepoResult(t *testing.T) {
	folderName := "Documents"
	blobs := []favorites.FavoriteBlob{
		{Blob: entity.Blob{FileName: "a.pdf"}, FolderName: &folderName},
		{Blob: entity.Blob{FileName: "b.pdf"}, FolderName: nil},
	}
	svc := newFavSvc(&mockFavRepo{listBlobsResult: blobs})

	got, err := svc.ListBlobs(bgCtx, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("blobs: got %d, want 2", len(got))
	}
	if got[0].FileName != "a.pdf" {
		t.Errorf("first blob name: got %q", got[0].FileName)
	}
	if got[1].FolderName != nil {
		t.Error("expected nil FolderName for second blob")
	}
}

func TestListBlobs_RepoError_Propagates(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{listBlobsErr: errors.New("db down")})
	_, err := svc.ListBlobs(bgCtx, uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListBlobs_EmptyResult(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{listBlobsResult: nil})
	got, err := svc.ListBlobs(bgCtx, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d", len(got))
	}
}

// ─── ListFolders ──────────────────────────────────────────────────────────────

func TestListFolders_ReturnsRepoResult(t *testing.T) {
	folders := []entity.Folder{
		{Name: "Work"},
		{Name: "Personal"},
	}
	svc := newFavSvc(&mockFavRepo{listFoldersResult: folders})

	got, err := svc.ListFolders(bgCtx, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("folders: got %d, want 2", len(got))
	}
	if got[0].Name != "Work" {
		t.Errorf("first folder name: got %q", got[0].Name)
	}
}

func TestListFolders_RepoError_Propagates(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{listFoldersErr: errors.New("db down")})
	_, err := svc.ListFolders(bgCtx, uuid.New())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListFolders_EmptyResult(t *testing.T) {
	svc := newFavSvc(&mockFavRepo{listFoldersResult: nil})
	got, err := svc.ListFolders(bgCtx, uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d", len(got))
	}
}
