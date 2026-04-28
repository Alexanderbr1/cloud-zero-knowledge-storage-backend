package v1

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	storageuc "cloud-backend/internal/usecase/storage"
)

func storagePresignPut(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		var in dto.StoragePresignPutRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		in.ContentType = strings.TrimSpace(in.ContentType)
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}
		encryptedFileKey, err := base64.StdEncoding.DecodeString(in.EncryptedFileKey)
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid encrypted_file_key")
			return
		}
		fileIV, err := base64.StdEncoding.DecodeString(in.FileIV)
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid file_iv")
			return
		}
		out, err := d.Storage.PresignPut(r.Context(), storageuc.PresignPutParams{
			UserID: uid, FileName: in.FileName, ContentType: in.ContentType,
			EncryptedFileKey: encryptedFileKey, FileIV: fileIV,
		})
		if err != nil {
			writeStorageErr(w, err)
			return
		}
		restapi.WriteJSON(w, http.StatusOK, dto.StoragePresignPutResponse{
			BlobID:       out.BlobID.String(),
			UploadURL:    out.UploadURL,
			ExpiresIn:    out.ExpiresIn,
			HTTPMethod:   out.HTTPMethod,
			ContentType:  out.ContentType,
			Instructions: fmt.Sprintf("PUT encrypted file bytes to upload_url; set header Content-Type: %s", out.ContentType),
		})
	}
}

func storagePresignGet(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		blobID, err := uuid.Parse(chi.URLParam(r, "blobID"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid blob_id")
			return
		}
		out, err := d.Storage.PresignGet(r.Context(), uid, blobID)
		if err != nil {
			writeStorageErr(w, err)
			return
		}
		restapi.WriteJSON(w, http.StatusOK, dto.StoragePresignGetResponse{
			BlobID:           out.BlobID.String(),
			DownloadURL:      out.DownloadURL,
			ExpiresIn:        out.ExpiresIn,
			HTTPMethod:       out.HTTPMethod,
			ContentType:      out.ContentType,
			EncryptedFileKey: base64.StdEncoding.EncodeToString(out.EncryptedFileKey),
			FileIV:           base64.StdEncoding.EncodeToString(out.FileIV),
		})
	}
}

func storageDeleteBlob(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		blobID, err := uuid.Parse(chi.URLParam(r, "blobID"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid blob_id")
			return
		}
		if err := d.Storage.DeleteBlob(r.Context(), uid, blobID); err != nil {
			writeStorageErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func storageListBlobs(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		blobs, err := d.Storage.ListBlobs(r.Context(), uid)
		if err != nil {
			writeStorageErr(w, err)
			return
		}
		items := make([]dto.StorageBlobItem, 0, len(blobs))
		for _, b := range blobs {
			items = append(items, dto.StorageBlobItem{
				BlobID:           b.ID.String(),
				FileName:         b.FileName,
				ContentType:      b.ContentType,
				CreatedAt:        b.CreatedAt,
				EncryptedFileKey: base64.StdEncoding.EncodeToString(b.EncryptedFileKey),
				FileIV:           base64.StdEncoding.EncodeToString(b.FileIV),
			})
		}
		restapi.WriteJSON(w, http.StatusOK, dto.StorageListBlobsResponse{Items: items})
	}
}

func writeStorageErr(w http.ResponseWriter, err error) {
	if errors.Is(err, storageuc.ErrNotFound) {
		restapi.WriteError(w, http.StatusNotFound, "not found")
	} else {
		restapi.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
