package v1_test

import (
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
	storageuc "cloud-backend/internal/usecase/storage"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func validCreateShareBody() string {
	eph := base64.StdEncoding.EncodeToString(make([]byte, 91))
	wfk := base64.StdEncoding.EncodeToString(make([]byte, 40))
	return `{"recipient_email":"bob@example.com","ephemeral_pub":"` + eph +
		`","wrapped_file_key":"` + wfk + `"}`
}

// ─── GET /storage/usage ──────────────────────────────────────────────────────

func TestStorageUsage_NoAuth_Returns401(t *testing.T) {
	w := do(fullRouter(routerOpts{}), http.MethodGet, "/storage/usage", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestStorageUsage_HappyPath(t *testing.T) {
	blobs := &mockBlobService{usageResult: storageuc.StorageUsage{UsedBytes: 500, QuotaBytes: 1000}}
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodGet, "/storage/usage", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	if body["used_bytes"] == nil {
		t.Error("used_bytes must be present")
	}
	if body["quota_bytes"] == nil {
		t.Error("quota_bytes must be present")
	}
}

func TestStorageUsage_ServiceError_Returns500(t *testing.T) {
	blobs := &mockBlobService{usageErr: errors.New("db down")}
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodGet, "/storage/usage", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── GET /storage/blobs ──────────────────────────────────────────────────────

func TestListBlobs_HappyPath_NoFilter(t *testing.T) {
	blob := entity.Blob{
		ID: uuid.New(), FileName: "a.txt", ContentType: "text/plain",
		EncryptedFileKey: make([]byte, 32),
		CreatedAt:        time.Now(),
	}
	blobs := &mockBlobService{blobs: []entity.Blob{blob}}
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodGet, "/storage/blobs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Errorf("want 1 item, got %d", len(items))
	}
}

func TestListBlobs_FolderRoot(t *testing.T) {
	blobs := &mockBlobService{blobs: []entity.Blob{}}
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodGet, "/storage/blobs?folder_id=root", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestListBlobs_SpecificFolderID(t *testing.T) {
	blobs := &mockBlobService{blobs: []entity.Blob{}}
	url := "/storage/blobs?folder_id=" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodGet, url, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestListBlobs_InvalidFolderID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodGet, "/storage/blobs?folder_id=not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ─── POST /storage/blobs/{blobID}/presign-get ────────────────────────────────

func TestPresignGet_HappyPath(t *testing.T) {
	blobs := &mockBlobService{
		presignGetOut: &storageuc.PresignGetResult{
			BlobID:           uuid.New(),
			DownloadURL:      "https://s3.example.com/dl",
			ExpiresIn:        3600,
			HTTPMethod:       "GET",
			ContentType:      "text/plain",
			EncryptedFileKey: make([]byte, 32),
			FileName:         "test.txt",
		},
	}
	url := "/storage/blobs/" + uuid.New().String() + "/presign-get"
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodPost, url, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	if body["download_url"] == "" {
		t.Error("download_url must be present")
	}
}

func TestPresignGet_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, "/storage/blobs/not-a-uuid/presign-get", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestPresignGet_NotFound_Returns404(t *testing.T) {
	blobs := &mockBlobService{presignGetErr: storageuc.ErrNotFound}
	url := "/storage/blobs/" + uuid.New().String() + "/presign-get"
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodPost, url, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// ─── DELETE /storage/blobs/{blobID} ──────────────────────────────────────────

func TestDeleteBlob_HappyPath(t *testing.T) {
	blobs := &mockBlobService{deleteFileName: "test.txt"}
	url := "/storage/blobs/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodDelete, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
}

func TestDeleteBlob_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, "/storage/blobs/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestDeleteBlob_NotFound_Returns404(t *testing.T) {
	blobs := &mockBlobService{deleteErr: storageuc.ErrNotFound}
	url := "/storage/blobs/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodDelete, url, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// ─── PATCH /storage/blobs/{blobID} (rename) ──────────────────────────────────

func TestRenameBlob_HappyPath(t *testing.T) {
	url := "/storage/blobs/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPatch, url, `{"name":"new-name.txt"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestRenameBlob_BlankName_Returns400(t *testing.T) {
	url := "/storage/blobs/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPatch, url, `{"name":"   "}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for blank name, got %d", w.Code)
	}
}

func TestRenameBlob_MissingName_Returns400(t *testing.T) {
	url := "/storage/blobs/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPatch, url, `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestRenameBlob_NotFound_Returns404(t *testing.T) {
	blobs := &mockBlobService{renameErr: storageuc.ErrNotFound}
	url := "/storage/blobs/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodPatch, url, `{"name":"x.txt"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// ─── PATCH /storage/blobs/{blobID}/folder (move blob) ────────────────────────

func TestMoveBlob_HappyPath_ToFolder(t *testing.T) {
	folders := &mockFolderService{moveBlobInfo: storageuc.MoveBlobInfo{FileName: "f.txt"}}
	url := "/storage/blobs/" + uuid.New().String() + "/folder"
	body := `{"folder_id":"` + uuid.New().String() + `"}`
	w := doWithAuth(fullRouter(routerOpts{folders: folders}), http.MethodPatch, url, body)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestMoveBlob_HappyPath_ToRoot(t *testing.T) {
	url := "/storage/blobs/" + uuid.New().String() + "/folder"
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPatch, url, `{}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestMoveBlob_InvalidFolderID_Returns400(t *testing.T) {
	url := "/storage/blobs/" + uuid.New().String() + "/folder"
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPatch, url, `{"folder_id":"not-a-uuid"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestMoveBlob_InvalidBlobUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPatch, "/storage/blobs/not-a-uuid/folder", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ─── GET /storage/search ─────────────────────────────────────────────────────

func TestSearch_HappyPath(t *testing.T) {
	blobs := &mockBlobService{searchResult: storageuc.SearchResult{}}
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodGet, "/storage/search?q=test", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
}

func TestSearch_MissingQuery_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodGet, "/storage/search", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for empty q, got %d", w.Code)
	}
}

func TestSearch_BlankQuery_Returns400(t *testing.T) {
	// spaces (%20) must be trimmed by the handler — result should be empty string → 400
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodGet, "/storage/search?q=%20%20%20", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for blank q, got %d", w.Code)
	}
}

func TestSearch_ServiceError_Returns500(t *testing.T) {
	blobs := &mockBlobService{searchErr: errors.New("db error")}
	w := doWithAuth(fullRouter(routerOpts{blobs: blobs}), http.MethodGet, "/storage/search?q=test", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}

// ─── POST /storage/folders ───────────────────────────────────────────────────

func TestCreateFolder_HappyPath(t *testing.T) {
	folder := entity.Folder{ID: uuid.New(), Name: "docs", CreatedAt: time.Now()}
	folders := &mockFolderService{createFolder: folder}
	w := doWithAuth(fullRouter(routerOpts{folders: folders}), http.MethodPost, "/storage/folders",
		`{"name":"docs"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	if body["folder_id"] == "" {
		t.Error("folder_id must be present")
	}
}

func TestCreateFolder_BlankName_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, "/storage/folders", `{"name":"   "}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCreateFolder_MissingName_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, "/storage/folders", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCreateFolder_InvalidParentID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, "/storage/folders",
		`{"name":"x","parent_id":"not-a-uuid"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCreateFolder_Conflict_Returns409(t *testing.T) {
	folders := &mockFolderService{createErr: storageuc.ErrFolderConflict}
	w := doWithAuth(fullRouter(routerOpts{folders: folders}), http.MethodPost, "/storage/folders",
		`{"name":"docs"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", w.Code)
	}
}

// ─── GET /storage/folders ────────────────────────────────────────────────────

func TestListFolders_HappyPath(t *testing.T) {
	folders := &mockFolderService{listFolders: []entity.Folder{}}
	w := doWithAuth(fullRouter(routerOpts{folders: folders}), http.MethodGet, "/storage/folders", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestListFolders_InvalidParentID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodGet, "/storage/folders?parent_id=bad", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ─── GET /storage/folders/{folderID} ─────────────────────────────────────────

func TestGetFolder_HappyPath(t *testing.T) {
	folder := entity.Folder{ID: uuid.New(), Name: "docs", CreatedAt: time.Now()}
	folders := &mockFolderService{getFolder: folder}
	url := "/storage/folders/" + folder.ID.String()
	w := doWithAuth(fullRouter(routerOpts{folders: folders}), http.MethodGet, url, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
}

func TestGetFolder_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodGet, "/storage/folders/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestGetFolder_NotFound_Returns404(t *testing.T) {
	folders := &mockFolderService{getErr: storageuc.ErrFolderNotFound}
	url := "/storage/folders/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{folders: folders}), http.MethodGet, url, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

// ─── PATCH /storage/folders/{folderID} (rename) ──────────────────────────────

func TestRenameFolder_HappyPath(t *testing.T) {
	folder := entity.Folder{ID: uuid.New(), Name: "renamed", CreatedAt: time.Now()}
	folders := &mockFolderService{renameFolder: folder}
	url := "/storage/folders/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{folders: folders}), http.MethodPatch, url, `{"name":"renamed"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
}

func TestRenameFolder_BlankName_Returns400(t *testing.T) {
	url := "/storage/folders/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPatch, url, `{"name":"  "}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestRenameFolder_Conflict_Returns409(t *testing.T) {
	folders := &mockFolderService{renameErr: storageuc.ErrFolderConflict}
	url := "/storage/folders/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{folders: folders}), http.MethodPatch, url, `{"name":"x"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", w.Code)
	}
}

// ─── PATCH /storage/folders/{folderID}/move ──────────────────────────────────

func TestMoveFolder_HappyPath(t *testing.T) {
	url := "/storage/folders/" + uuid.New().String() + "/move"
	body := `{"parent_id":"` + uuid.New().String() + `"}`
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPatch, url, body)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestMoveFolder_ToRoot(t *testing.T) {
	url := "/storage/folders/" + uuid.New().String() + "/move"
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPatch, url, `{}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestMoveFolder_Cycle_Returns422(t *testing.T) {
	folders := &mockFolderService{moveFolderErr: storageuc.ErrFolderCycle}
	url := "/storage/folders/" + uuid.New().String() + "/move"
	w := doWithAuth(fullRouter(routerOpts{folders: folders}), http.MethodPatch, url, `{}`)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", w.Code)
	}
}

func TestMoveFolder_InvalidParentID_Returns400(t *testing.T) {
	url := "/storage/folders/" + uuid.New().String() + "/move"
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPatch, url, `{"parent_id":"bad"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ─── DELETE /storage/folders/{folderID} ──────────────────────────────────────

func TestDeleteFolder_HappyPath(t *testing.T) {
	folders := &mockFolderService{deleteFolderName: "docs"}
	url := "/storage/folders/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{folders: folders}), http.MethodDelete, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
}

func TestDeleteFolder_NotEmpty_Returns409(t *testing.T) {
	folders := &mockFolderService{deleteFolderErr: storageuc.ErrFolderNotEmpty}
	url := "/storage/folders/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{folders: folders}), http.MethodDelete, url, "")
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", w.Code)
	}
}

func TestDeleteFolder_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, "/storage/folders/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}
