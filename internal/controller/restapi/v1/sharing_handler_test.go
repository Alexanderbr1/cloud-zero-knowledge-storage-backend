package v1_test

import (
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
	sharinguc "cloud-backend/internal/usecase/sharing"
)

// ─── GET /users/public-key ───────────────────────────────────────────────────

func TestGetPublicKey_HappyPath(t *testing.T) {
	sharing := &mockSharingService{publicKey: make([]byte, 91)}
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}),
		http.MethodGet, "/users/public-key?email=bob@example.com", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	if body["public_key"] == "" {
		t.Error("public_key must be present")
	}
}

func TestGetPublicKey_MissingEmail_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodGet, "/users/public-key", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestGetPublicKey_NotFound_Returns404(t *testing.T) {
	sharing := &mockSharingService{publicKeyErr: errors.New("not found")}
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}),
		http.MethodGet, "/users/public-key?email=ghost@example.com", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestGetPublicKey_NoAuth_Returns401(t *testing.T) {
	w := do(fullRouter(routerOpts{}), http.MethodGet, "/users/public-key?email=bob@example.com", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

// ─── POST /storage/blobs/{blobID}/shares ─────────────────────────────────────

func TestCreateShare_HappyPath(t *testing.T) {
	shareView := entity.FileShareView{
		FileShare: entity.FileShare{
			ID:             uuid.New(),
			BlobID:         uuid.New(),
			OwnerID:        uuid.New(),
			RecipientID:    uuid.New(),
			EphemeralPub:   make([]byte, 91),
			WrappedFileKey: make([]byte, 40),
			CreatedAt:      time.Now(),
		},
	}
	sharing := &mockSharingService{createShare: shareView}
	url := "/storage/blobs/" + uuid.New().String() + "/shares"
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodPost, url, validCreateShareBody())
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	if body["share_id"] == "" {
		t.Error("share_id must be present")
	}
}

func TestCreateShare_InvalidBlobUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, "/storage/blobs/not-a-uuid/shares", validCreateShareBody())
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCreateShare_InvalidJSON_Returns400(t *testing.T) {
	url := "/storage/blobs/" + uuid.New().String() + "/shares"
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, url, `{bad}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCreateShare_EphemeralPubWrongLength_Returns400(t *testing.T) {
	// ephemeral_pub must be exactly 91 bytes; provide 90
	eph := base64.StdEncoding.EncodeToString(make([]byte, 90))
	wfk := base64.StdEncoding.EncodeToString(make([]byte, 40))
	body := `{"recipient_email":"bob@example.com","ephemeral_pub":"` + eph +
		`","wrapped_file_key":"` + wfk + `"}`
	url := "/storage/blobs/" + uuid.New().String() + "/shares"
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, url, body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCreateShare_WrappedFileKeyWrongLength_Returns400(t *testing.T) {
	// wrapped_file_key must be exactly 40 bytes; provide 39
	eph := base64.StdEncoding.EncodeToString(make([]byte, 91))
	wfk := base64.StdEncoding.EncodeToString(make([]byte, 39))
	body := `{"recipient_email":"bob@example.com","ephemeral_pub":"` + eph +
		`","wrapped_file_key":"` + wfk + `"}`
	url := "/storage/blobs/" + uuid.New().String() + "/shares"
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, url, body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCreateShare_InvalidExpiresAt_Returns400(t *testing.T) {
	eph := base64.StdEncoding.EncodeToString(make([]byte, 91))
	wfk := base64.StdEncoding.EncodeToString(make([]byte, 40))
	body := `{"recipient_email":"bob@example.com","ephemeral_pub":"` + eph +
		`","wrapped_file_key":"` + wfk + `","expires_at":"not-a-date"}`
	url := "/storage/blobs/" + uuid.New().String() + "/shares"
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodPost, url, body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCreateShare_SelfShare_Returns400(t *testing.T) {
	sharing := &mockSharingService{createShareErr: sharinguc.ErrSelfShare}
	url := "/storage/blobs/" + uuid.New().String() + "/shares"
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodPost, url, validCreateShareBody())
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestCreateShare_DuplicateShare_Returns409(t *testing.T) {
	sharing := &mockSharingService{createShareErr: sharinguc.ErrDuplicateShare}
	url := "/storage/blobs/" + uuid.New().String() + "/shares"
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodPost, url, validCreateShareBody())
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", w.Code)
	}
}

func TestCreateShare_NoPublicKey_Returns422(t *testing.T) {
	sharing := &mockSharingService{createShareErr: sharinguc.ErrNoPublicKey}
	url := "/storage/blobs/" + uuid.New().String() + "/shares"
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodPost, url, validCreateShareBody())
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", w.Code)
	}
}

// ─── GET /storage/blobs/{blobID}/shares ──────────────────────────────────────

func TestListMyShares_HappyPath(t *testing.T) {
	sharing := &mockSharingService{myShares: []entity.FileShareView{}}
	url := "/storage/blobs/" + uuid.New().String() + "/shares"
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodGet, url, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
}

func TestListMyShares_InvalidBlobUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodGet, "/storage/blobs/not-a-uuid/shares", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

// ─── GET /shares/incoming ────────────────────────────────────────────────────

func TestListSharedWithMe_HappyPath(t *testing.T) {
	sharing := &mockSharingService{sharedWithMe: []entity.FileShareView{}}
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodGet, "/shares/incoming", "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestListSharedWithMe_NoAuth_Returns401(t *testing.T) {
	w := do(fullRouter(routerOpts{}), http.MethodGet, "/shares/incoming", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

// ─── GET /shares/{shareID} ───────────────────────────────────────────────────

func TestGetSharedFile_HappyPath(t *testing.T) {
	result := sharinguc.SharedFileResult{
		Share: entity.FileShareView{
			FileShare: entity.FileShare{
				ID:             uuid.New(),
				BlobID:         uuid.New(),
				OwnerID:        uuid.New(),
				EphemeralPub:   make([]byte, 91),
				WrappedFileKey: make([]byte, 40),
				CreatedAt:      time.Now(),
			},
		},
		DownloadURL: "https://s3.example.com/dl",
		FileIV:      make([]byte, 12),
	}
	sharing := &mockSharingService{sharedFile: result}
	url := "/shares/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodGet, url, "")
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body)
	}
	var body map[string]any
	decodeBody(t, w, &body)
	if body["download_url"] == "" {
		t.Error("download_url must be present")
	}
}

func TestGetSharedFile_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodGet, "/shares/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestGetSharedFile_NotFound_Returns404(t *testing.T) {
	sharing := &mockSharingService{sharedFileErr: sharinguc.ErrNotFound}
	url := "/shares/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodGet, url, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestGetSharedFile_Forbidden_Returns403(t *testing.T) {
	sharing := &mockSharingService{sharedFileErr: sharinguc.ErrForbidden}
	url := "/shares/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodGet, url, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", w.Code)
	}
}

// ErrExpired and ErrRevoked now both return 403 (unified "forbidden") to avoid
// leaking share state information to callers who are not the intended recipient.
func TestGetSharedFile_Expired_Returns403(t *testing.T) {
	sharing := &mockSharingService{sharedFileErr: sharinguc.ErrExpired}
	url := "/shares/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodGet, url, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", w.Code)
	}
}

func TestGetSharedFile_Revoked_Returns403(t *testing.T) {
	sharing := &mockSharingService{sharedFileErr: sharinguc.ErrRevoked}
	url := "/shares/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodGet, url, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", w.Code)
	}
}

// ─── DELETE /shares/{shareID} ────────────────────────────────────────────────

func TestRevokeShare_HappyPath(t *testing.T) {
	url := "/shares/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, url, "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", w.Code, w.Body)
	}
}

func TestRevokeShare_InvalidUUID_Returns400(t *testing.T) {
	w := doWithAuth(fullRouter(routerOpts{}), http.MethodDelete, "/shares/not-a-uuid", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestRevokeShare_NotFound_Returns404(t *testing.T) {
	sharing := &mockSharingService{revokeErr: sharinguc.ErrNotFound}
	url := "/shares/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodDelete, url, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestRevokeShare_Forbidden_Returns403(t *testing.T) {
	sharing := &mockSharingService{revokeErr: sharinguc.ErrForbidden}
	url := "/shares/" + uuid.New().String()
	w := doWithAuth(fullRouter(routerOpts{sharing: sharing}), http.MethodDelete, url, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", w.Code)
	}
}
