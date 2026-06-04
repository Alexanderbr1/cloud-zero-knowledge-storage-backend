package v1_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
	favoritesuc "cloud-backend/internal/usecase/favorites"
)

// ─── GET /favorites ──────────────────────────────────────────────────────────

func TestFavoritesList_HappyPath(t *testing.T) {
	folderName := "docs"
	favs := &mockFavoritesService{
		blobs: []favoritesuc.FavoriteBlob{
			{
				Blob: entity.Blob{
					ID:               uuid.New(),
					FileName:         "report.pdf",
					ContentType:      "application/pdf",
					EncryptedFileKey: make([]byte, 32),
					CreatedAt:        time.Now(),
				},
				FolderName: &folderName,
			},
		},
		folders: []entity.Folder{
			{ID: uuid.New(), Name: "docs", CreatedAt: time.Now()},
		},
	}
	w := doWithAuth(fullRouter(routerOpts{favorites: favs}), http.MethodGet, "/favorites/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	blobs, _ := body["blobs"].([]any)
	if len(blobs) != 1 {
		t.Errorf("want 1 blob, got %d", len(blobs))
	}
	folders, _ := body["folders"].([]any)
	if len(folders) != 1 {
		t.Errorf("want 1 folder, got %d", len(folders))
	}
}

func TestFavoritesList_NoAuth_Returns401(t *testing.T) {
	w := do(fullRouter(routerOpts{}), http.MethodGet, "/favorites/", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestFavoritesList_BlobsError_Returns500(t *testing.T) {
	favs := &mockFavoritesService{listBlobsErr: errors.New("db down")}
	w := doWithAuth(fullRouter(routerOpts{favorites: favs}), http.MethodGet, "/favorites/", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

func TestFavoritesList_FoldersError_Returns500(t *testing.T) {
	favs := &mockFavoritesService{listFoldersErr: errors.New("db down")}
	w := doWithAuth(fullRouter(routerOpts{favorites: favs}), http.MethodGet, "/favorites/", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── POST /favorites/blobs/{id} ──────────────────────────────────────────────

func TestFavoritesAddBlob_HappyPath(t *testing.T) {
	url := "/favorites/blobs/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestFavoritesAddBlob_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, "/favorites/blobs/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestFavoritesAddBlob_ServiceError_Returns500(t *testing.T) {
	favs := &mockFavoritesService{addBlobErr: errors.New("db error")}
	url := "/favorites/blobs/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{favorites: favs}), http.MethodPost, url, "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── DELETE /favorites/blobs/{id} ────────────────────────────────────────────

func TestFavoritesRemoveBlob_HappyPath(t *testing.T) {
	url := "/favorites/blobs/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestFavoritesRemoveBlob_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, "/favorites/blobs/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ─── POST /favorites/folders/{id} ────────────────────────────────────────────

func TestFavoritesAddFolder_HappyPath(t *testing.T) {
	url := "/favorites/folders/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestFavoritesAddFolder_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, "/favorites/folders/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestFavoritesAddFolder_ServiceError_Returns500(t *testing.T) {
	favs := &mockFavoritesService{addFolderErr: errors.New("db error")}
	url := "/favorites/folders/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{favorites: favs}), http.MethodPost, url, "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── DELETE /favorites/folders/{id} ──────────────────────────────────────────

func TestFavoritesRemoveFolder_HappyPath(t *testing.T) {
	url := "/favorites/folders/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestFavoritesRemoveFolder_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, "/favorites/folders/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}
