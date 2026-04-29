package v1

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	"cloud-backend/internal/entity"
	sharinguc "cloud-backend/internal/usecase/sharing"
)

const (
	// P-256 SPKI public key is always exactly 91 bytes.
	ecSpkiLen = 91
	// AES-256 key wrapped with AES-KW is always 40 bytes.
	aesKwWrappedLen = 40
)

func getRecipientPublicKey(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		email := r.URL.Query().Get("email")
		if email == "" {
			restapi.WriteError(w, http.StatusBadRequest, "email query param required")
			return
		}
		pub, err := d.Sharing.GetRecipientPublicKey(r.Context(), email)
		if err != nil {
			// Always 404 — don't distinguish "user not found" from "user has no key"
			// so callers cannot tell whether an email is registered.
			restapi.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		restapi.WriteJSON(w, http.StatusOK, dto.GetPublicKeyResponse{
			PublicKey: base64.StdEncoding.EncodeToString(pub),
		})
	}
}

func createShare(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerID, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		blobID, err := uuid.Parse(chi.URLParam(r, "blobID"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid blob_id")
			return
		}

		var in dto.CreateShareRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}

		ephemeralPub, err := base64.StdEncoding.DecodeString(in.EphemeralPub)
		if err != nil || len(ephemeralPub) != ecSpkiLen {
			restapi.WriteError(w, http.StatusBadRequest, "invalid ephemeral_pub")
			return
		}
		wrappedFileKey, err := base64.StdEncoding.DecodeString(in.WrappedFileKey)
		if err != nil || len(wrappedFileKey) != aesKwWrappedLen {
			restapi.WriteError(w, http.StatusBadRequest, "invalid wrapped_file_key")
			return
		}

		var expiresAt *time.Time
		if in.ExpiresAt != nil {
			t, err := time.Parse(time.RFC3339, *in.ExpiresAt)
			if err != nil {
				restapi.WriteError(w, http.StatusBadRequest, "invalid expires_at (must be RFC3339)")
				return
			}
			expiresAt = &t
		}

		share, err := d.Sharing.CreateShare(r.Context(), sharinguc.CreateShareParams{
			BlobID:         blobID,
			OwnerID:        ownerID,
			RecipientEmail: strings.TrimSpace(strings.ToLower(in.RecipientEmail)),
			EphemeralPub:   ephemeralPub,
			WrappedFileKey: wrappedFileKey,
			ExpiresAt:      expiresAt,
		})
		if err != nil {
			writeSharingErr(w, err)
			return
		}
		restapi.WriteJSON(w, http.StatusCreated, shareToDTO(share, "", ""))
	}
}

func listSharedWithMe(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		shares, err := d.Sharing.ListSharedWithMe(r.Context(), uid)
		if err != nil {
			writeSharingErr(w, err)
			return
		}
		items := make([]dto.ShareResponse, 0, len(shares))
		for _, s := range shares {
			items = append(items, shareToDTO(s, "", ""))
		}
		restapi.WriteJSON(w, http.StatusOK, dto.ListSharesResponse{Items: items})
	}
}

func listMyShares(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerID, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		blobID, err := uuid.Parse(chi.URLParam(r, "blobID"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid blob_id")
			return
		}
		shares, err := d.Sharing.ListMyShares(r.Context(), blobID, ownerID)
		if err != nil {
			writeSharingErr(w, err)
			return
		}
		items := make([]dto.ShareResponse, 0, len(shares))
		for _, s := range shares {
			items = append(items, shareToDTO(s, "", ""))
		}
		restapi.WriteJSON(w, http.StatusOK, dto.ListSharesResponse{Items: items})
	}
}

func getSharedFile(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		shareID, err := uuid.Parse(chi.URLParam(r, "shareID"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid share_id")
			return
		}
		result, err := d.Sharing.GetSharedFile(r.Context(), shareID, uid)
		if err != nil {
			writeSharingErr(w, err)
			return
		}
		restapi.WriteJSON(w, http.StatusOK,
			shareToDTO(result.Share, result.DownloadURL, base64.StdEncoding.EncodeToString(result.FileIV)))
	}
}

func revokeShare(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerID, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		shareID, err := uuid.Parse(chi.URLParam(r, "shareID"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid share_id")
			return
		}
		if err := d.Sharing.RevokeShare(r.Context(), shareID, ownerID); err != nil {
			writeSharingErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func shareToDTO(s entity.FileShare, downloadURL, fileIVB64 string) dto.ShareResponse {
	resp := dto.ShareResponse{
		ShareID:         s.ID.String(),
		BlobID:          s.BlobID.String(),
		OwnerEmail:      s.OwnerEmail,
		RecipientEmail:  s.RecipientEmail,
		BlobFileName:    s.BlobFileName,
		BlobContentType: s.BlobContentType,
		EphemeralPub:    base64.StdEncoding.EncodeToString(s.EphemeralPub),
		WrappedFileKey:  base64.StdEncoding.EncodeToString(s.WrappedFileKey),
		CreatedAt:       s.CreatedAt,
		DownloadURL:     downloadURL,
		FileIV:          fileIVB64,
	}
	if s.ExpiresAt != nil {
		ts := s.ExpiresAt.Format(time.RFC3339)
		resp.ExpiresAt = &ts
	}
	return resp
}

func writeSharingErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sharinguc.ErrNotFound):
		restapi.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, sharinguc.ErrForbidden):
		restapi.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, sharinguc.ErrNoPublicKey):
		restapi.WriteError(w, http.StatusUnprocessableEntity, "recipient has no encryption key")
	case errors.Is(err, sharinguc.ErrExpired):
		restapi.WriteError(w, http.StatusGone, "share expired")
	case errors.Is(err, sharinguc.ErrRevoked):
		restapi.WriteError(w, http.StatusGone, "share revoked")
	case errors.Is(err, sharinguc.ErrDuplicateShare):
		restapi.WriteError(w, http.StatusConflict, "share already exists for this recipient")
	case errors.Is(err, sharinguc.ErrSelfShare):
		restapi.WriteError(w, http.StatusBadRequest, "cannot share a file with yourself")
	default:
		restapi.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
