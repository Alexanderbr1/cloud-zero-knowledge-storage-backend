package v1

import (
	"encoding/base64"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	"cloud-backend/internal/entity"
	storageuc "cloud-backend/internal/usecase/storage"
)

func trashList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		result, err := d.Storage.ListTrash(r.Context(), uid)
		if err != nil {
			restapi.WriteInternalError(w, d.Logger, err)
			return
		}
		restapi.WriteJSON(w, http.StatusOK, trashResultToDTO(result))
	}
}

func trashRestoreBlob(d Deps) http.HandlerFunc {
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
		if err := d.Storage.RestoreBlob(r.Context(), uid, blobID); err != nil {
			writeStorageErr(w, err, d.Logger)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func trashDeleteBlob(d Deps) http.HandlerFunc {
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
		if err := d.Storage.HardDeleteBlob(r.Context(), uid, blobID); err != nil {
			writeStorageErr(w, err, d.Logger)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func trashRestoreFolder(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		folderID, err := uuid.Parse(chi.URLParam(r, "folderID"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid folder_id")
			return
		}
		if err := d.Storage.RestoreFolder(r.Context(), uid, folderID); err != nil {
			writeStorageErr(w, err, d.Logger)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func trashDeleteFolder(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		folderID, err := uuid.Parse(chi.URLParam(r, "folderID"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid folder_id")
			return
		}
		if err := d.Storage.HardDeleteFolder(r.Context(), uid, folderID); err != nil {
			writeStorageErr(w, err, d.Logger)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func trashEmpty(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		if err := d.Storage.EmptyTrash(r.Context(), uid); err != nil {
			restapi.WriteInternalError(w, d.Logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func trashResultToDTO(r storageuc.TrashResult) dto.TrashListResponse {
	blobs := make([]dto.TrashBlobItem, 0, len(r.Blobs))
	for _, b := range r.Blobs {
		blobs = append(blobs, blobToTrashDTO(b))
	}
	folders := make([]dto.TrashFolderItem, 0, len(r.Folders))
	for _, f := range r.Folders {
		folders = append(folders, folderToTrashDTO(f))
	}
	return dto.TrashListResponse{Blobs: blobs, Folders: folders}
}

func blobToTrashDTO(b entity.Blob) dto.TrashBlobItem {
	item := dto.TrashBlobItem{
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

func folderToTrashDTO(f entity.Folder) dto.TrashFolderItem {
	item := dto.TrashFolderItem{
		FolderID:  f.ID.String(),
		Name:      f.Name,
		CreatedAt: f.CreatedAt,
	}
	if f.ParentID != nil {
		s := f.ParentID.String()
		item.ParentID = &s
	}
	return item
}
