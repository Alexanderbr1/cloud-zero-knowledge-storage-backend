package v1

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	"cloud-backend/internal/entity"
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
		var folderID *uuid.UUID
		if in.FolderID != nil {
			parsed, err := uuid.Parse(*in.FolderID)
			if err != nil {
				restapi.WriteError(w, http.StatusBadRequest, "invalid folder_id")
				return
			}
			folderID = &parsed
		}
		out, err := d.Storage.PresignPut(r.Context(), storageuc.PresignPutParams{
			UserID: uid, FileName: in.FileName, ContentType: in.ContentType,
			FileSize: in.FileSize, EncryptedFileKey: encryptedFileKey,
			FileIV: fileIV, FolderID: folderID,
		})
		if err != nil {
			writeStorageErr(w, err, d.Logger)
			return
		}
		restapi.WriteJSON(w, http.StatusOK, dto.StoragePresignPutResponse{
			BlobID:      out.BlobID.String(),
			UploadURL:   out.UploadURL,
			ExpiresIn:   out.ExpiresIn,
			HTTPMethod:  out.HTTPMethod,
			ContentType: out.ContentType,
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
			writeStorageErr(w, err, d.Logger)
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
			writeStorageErr(w, err, d.Logger)
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

		var (
			blobs []entity.Blob
			err   error
		)
		folderRaw := r.URL.Query().Get("folder_id")
		switch folderRaw {
		case "":
			blobs, err = d.Storage.ListBlobs(r.Context(), uid)
		case "root":
			blobs, err = d.Storage.ListBlobsInFolder(r.Context(), uid, nil)
		default:
			folderID, parseErr := uuid.Parse(folderRaw)
			if parseErr != nil {
				restapi.WriteError(w, http.StatusBadRequest, "invalid folder_id")
				return
			}
			blobs, err = d.Storage.ListBlobsInFolder(r.Context(), uid, &folderID)
		}
		if err != nil {
			writeStorageErr(w, err, d.Logger)
			return
		}

		items := make([]dto.StorageBlobItem, 0, len(blobs))
		for _, b := range blobs {
			items = append(items, blobToDTO(b))
		}
		restapi.WriteJSON(w, http.StatusOK, dto.StorageListBlobsResponse{Items: items})
	}
}

func blobToDTO(b entity.Blob) dto.StorageBlobItem {
	item := dto.StorageBlobItem{
		BlobID:           b.ID.String(),
		FileName:         b.FileName,
		ContentType:      b.ContentType,
		FileSize:         b.FileSize,
		CreatedAt:        b.CreatedAt,
		EncryptedFileKey: base64.StdEncoding.EncodeToString(b.EncryptedFileKey),
		FileIV:           base64.StdEncoding.EncodeToString(b.FileIV),
	}
	if b.FolderID != nil {
		s := b.FolderID.String()
		item.FolderID = &s
	}
	return item
}

func renameBlob(d Deps) http.HandlerFunc {
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
		var in dto.RenameBlobRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			restapi.WriteError(w, http.StatusBadRequest, "name must not be blank")
			return
		}
		if err := d.Storage.RenameBlob(r.Context(), uid, blobID, name); err != nil {
			writeStorageErr(w, err, d.Logger)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func storageSearch(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			restapi.WriteError(w, http.StatusBadRequest, "q is required")
			return
		}
		result, err := d.Storage.Search(r.Context(), storageuc.SearchParams{UserID: uid, Query: q})
		if err != nil {
			restapi.WriteInternalError(w, d.Logger, err)
			return
		}

		blobs := make([]dto.SearchBlobItem, 0, len(result.Blobs))
		for _, b := range result.Blobs {
			blobs = append(blobs, searchBlobToDTO(b))
		}

		folders := make([]dto.FolderItem, 0, len(result.Folders))
		for _, f := range result.Folders {
			folders = append(folders, folderToDTO(f))
		}

		restapi.WriteJSON(w, http.StatusOK, dto.SearchResponse{Blobs: blobs, Folders: folders})
	}
}

func searchBlobToDTO(b storageuc.SearchBlobRecord) dto.SearchBlobItem {
	item := dto.SearchBlobItem{
		BlobID:           b.ID.String(),
		FileName:         b.FileName,
		ContentType:      b.ContentType,
		FileSize:         b.FileSize,
		CreatedAt:        b.CreatedAt,
		EncryptedFileKey: base64.StdEncoding.EncodeToString(b.EncryptedFileKey),
		FileIV:           base64.StdEncoding.EncodeToString(b.FileIV),
		FolderName:       b.FolderName,
	}
	if b.FolderID != nil {
		s := b.FolderID.String()
		item.FolderID = &s
	}
	return item
}

func writeStorageErr(w http.ResponseWriter, err error, log zerolog.Logger) {
	switch {
	case errors.Is(err, storageuc.ErrNotFound):
		restapi.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, storageuc.ErrFolderNotFound):
		restapi.WriteError(w, http.StatusNotFound, "folder not found")
	default:
		restapi.WriteInternalError(w, log, err)
	}
}
