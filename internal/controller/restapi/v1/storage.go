package v1

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	storageuc "cloud-backend/internal/usecase/storage"
)

func registerStorageRoutes(r chi.Router, d Deps) {
	r.Use(middleware.Timeout(30 * time.Minute))
	r.Post("/presign", storagePresignPut(d))
	r.Get("/blobs", storageListBlobs(d))
	r.Post("/blobs/{blobID}/presign-get", storagePresignGet(d))
	r.Delete("/blobs/{blobID}", storageDeleteBlob(d))
}

func storagePresignPut(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.UserIDFromContext(r.Context())
		if !ok || uid == uuid.Nil {
			restapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var in dto.StoragePresignPutRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}
		out, err := d.Storage.PresignPut(r.Context(), uid, in.FileName)
		if mapStorageErr(w, err) {
			return
		}
		restapi.WriteJSON(w, http.StatusOK, dto.StoragePresignPutResponse{
			BlobID:       out.BlobID.String(),
			UploadURL:    out.UploadURL,
			ExpiresIn:    out.ExpiresIn,
			HTTPMethod:   out.HTTPMethod,
			Instructions: "PUT file bytes to upload_url; use Content-Type: application/octet-stream",
		})
	}
}

func storagePresignGet(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.UserIDFromContext(r.Context())
		if !ok || uid == uuid.Nil {
			restapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		blobID, err := uuid.Parse(chi.URLParam(r, "blobID"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid blob_id")
			return
		}
		out, err := d.Storage.PresignGet(r.Context(), uid, blobID)
		if mapStorageErr(w, err) {
			return
		}
		restapi.WriteJSON(w, http.StatusOK, dto.StoragePresignGetResponse{
			BlobID:      out.BlobID.String(),
			DownloadURL: out.DownloadURL,
			ExpiresIn:   out.ExpiresIn,
			HTTPMethod:  out.HTTPMethod,
		})
	}
}

func storageDeleteBlob(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.UserIDFromContext(r.Context())
		if !ok || uid == uuid.Nil {
			restapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		blobID, err := uuid.Parse(chi.URLParam(r, "blobID"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid blob_id")
			return
		}
		if err := d.Storage.DeleteBlob(r.Context(), uid, blobID); mapStorageErr(w, err) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func storageListBlobs(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.UserIDFromContext(r.Context())
		if !ok || uid == uuid.Nil {
			restapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		blobs, err := d.Storage.ListBlobs(r.Context(), uid)
		if mapStorageErr(w, err) {
			return
		}
		items := make([]dto.StorageBlobItem, 0, len(blobs))
		for _, b := range blobs {
			items = append(items, dto.StorageBlobItem{
				BlobID:    b.ID.String(),
				FileName:  b.FileName,
				CreatedAt: b.CreatedAt,
			})
		}
		restapi.WriteJSON(w, http.StatusOK, dto.StorageListBlobsResponse{Items: items})
	}
}

func mapStorageErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, storageuc.ErrNotFound) {
		restapi.WriteError(w, http.StatusNotFound, "not found")
	} else {
		restapi.WriteError(w, http.StatusInternalServerError, "internal error")
	}
	return true
}
