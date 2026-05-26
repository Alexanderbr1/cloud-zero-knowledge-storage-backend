package v1_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	storageuc "cloud-backend/internal/usecase/storage"
)

// ─── GET /trash ──────────────────────────────────────────────────────────────

func TestTrashList_HappyPath(t *testing.T) {
	trash := &mockTrashService{trashResult: storageuc.TrashResult{}}
	w := doWithAuth(fullRouter(routerOpts{trash: trash}), http.MethodGet, "/trash/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	if body["blobs"] == nil {
		t.Error("blobs key must be present")
	}
	if body["folders"] == nil {
		t.Error("folders key must be present")
	}
}

func TestTrashList_NoAuth_Returns401(t *testing.T) {
	w := do(fullRouter(routerOpts{}), http.MethodGet, "/trash/", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestTrashList_ServiceError_Returns500(t *testing.T) {
	trash := &mockTrashService{trashListErr: errors.New("db down")}
	w := doWithAuth(fullRouter(routerOpts{trash: trash}), http.MethodGet, "/trash/", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── DELETE /trash (empty trash) ─────────────────────────────────────────────

func TestTrashEmpty_HappyPath(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, "/trash/", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestTrashEmpty_ServiceError_Returns500(t *testing.T) {
	trash := &mockTrashService{emptyErr: errors.New("db error")}
	w := doWithAuth(fullRouter(routerOpts{trash: trash}), http.MethodDelete, "/trash/", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── POST /trash/blobs/{blobID}/restore ──────────────────────────────────────

func TestTrashRestoreBlob_HappyPath(t *testing.T) {
	trash := &mockTrashService{restoreBlobName: "doc.pdf"}
	url := "/trash/blobs/" + uuid.New().String() + "/restore"
	w := doWithAuth(fullRouter(routerOpts{trash: trash}), http.MethodPost, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestTrashRestoreBlob_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, "/trash/blobs/not-a-uuid/restore", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestTrashRestoreBlob_NotFound_Returns404(t *testing.T) {
	trash := &mockTrashService{restoreBlobErr: storageuc.ErrNotFound}
	url := "/trash/blobs/" + uuid.New().String() + "/restore"
	w := doWithAuth(fullRouter(routerOpts{trash: trash}), http.MethodPost, url, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// ─── DELETE /trash/blobs/{blobID} ────────────────────────────────────────────

func TestTrashHardDeleteBlob_HappyPath(t *testing.T) {
	trash := &mockTrashService{hardDeleteBlobName: "doc.pdf"}
	url := "/trash/blobs/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{trash: trash}), http.MethodDelete, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestTrashHardDeleteBlob_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, "/trash/blobs/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestTrashHardDeleteBlob_NotFound_Returns404(t *testing.T) {
	trash := &mockTrashService{hardDeleteBlobErr: storageuc.ErrNotFound}
	url := "/trash/blobs/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{trash: trash}), http.MethodDelete, url, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// ─── POST /trash/folders/{folderID}/restore ───────────────────────────────────

func TestTrashRestoreFolder_HappyPath(t *testing.T) {
	trash := &mockTrashService{restoreFolderName: "archive"}
	url := "/trash/folders/" + uuid.New().String() + "/restore"
	w := doWithAuth(fullRouter(routerOpts{trash: trash}), http.MethodPost, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestTrashRestoreFolder_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, "/trash/folders/not-a-uuid/restore", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestTrashRestoreFolder_NotFound_Returns404(t *testing.T) {
	trash := &mockTrashService{restoreFolderErr: storageuc.ErrNotFound}
	url := "/trash/folders/" + uuid.New().String() + "/restore"
	w := doWithAuth(fullRouter(routerOpts{trash: trash}), http.MethodPost, url, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// ─── DELETE /trash/folders/{folderID} ────────────────────────────────────────

func TestTrashHardDeleteFolder_HappyPath(t *testing.T) {
	trash := &mockTrashService{hardDeleteFolderName: "archive"}
	url := "/trash/folders/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{trash: trash}), http.MethodDelete, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestTrashHardDeleteFolder_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, "/trash/folders/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestTrashHardDeleteFolder_NotFound_Returns404(t *testing.T) {
	trash := &mockTrashService{hardDeleteFolderErr: storageuc.ErrNotFound}
	url := "/trash/folders/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{trash: trash}), http.MethodDelete, url, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}
