package v1

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	"cloud-backend/internal/entity"
	storageuc "cloud-backend/internal/usecase/storage"
)

func createFolder(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		var in dto.CreateFolderRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		if err := restapi.ValidateStruct(&in); err != nil {
			restapi.WriteValidationError(w, err)
			return
		}
		var parentID *uuid.UUID
		if in.ParentID != nil {
			parsed, err := uuid.Parse(*in.ParentID)
			if err != nil {
				restapi.WriteError(w, http.StatusBadRequest, "invalid parent_id")
				return
			}
			parentID = &parsed
		}
		name := strings.TrimSpace(in.Name)
		if name == "" {
			restapi.WriteError(w, http.StatusBadRequest, "name must not be blank")
			return
		}
		folder, err := d.Storage.CreateFolder(r.Context(), storageuc.CreateFolderParams{
			UserID:   uid,
			ParentID: parentID,
			Name:     name,
		})
		if err != nil {
			writeFolderErr(w, err)
			return
		}
		restapi.WriteJSON(w, http.StatusCreated, folderToDTO(folder))
	}
}

func getFolder(d Deps) http.HandlerFunc {
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
		folder, err := d.Storage.GetFolder(r.Context(), uid, folderID)
		if err != nil {
			writeFolderErr(w, err)
			return
		}
		restapi.WriteJSON(w, http.StatusOK, folderToDTO(folder))
	}
}

func listFolders(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		var parentID *uuid.UUID
		if raw := r.URL.Query().Get("parent_id"); raw != "" {
			parsed, err := uuid.Parse(raw)
			if err != nil {
				restapi.WriteError(w, http.StatusBadRequest, "invalid parent_id")
				return
			}
			parentID = &parsed
		}
		folders, err := d.Storage.ListFolders(r.Context(), uid, parentID)
		if err != nil {
			writeFolderErr(w, err)
			return
		}
		items := make([]dto.FolderItem, 0, len(folders))
		for _, f := range folders {
			items = append(items, folderToDTO(f))
		}
		restapi.WriteJSON(w, http.StatusOK, dto.ListFoldersResponse{Items: items})
	}
}

func renameFolder(d Deps) http.HandlerFunc {
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
		var in dto.RenameFolderRequest
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
		folder, err := d.Storage.RenameFolder(r.Context(), uid, folderID, name)
		if err != nil {
			writeFolderErr(w, err)
			return
		}
		restapi.WriteJSON(w, http.StatusOK, folderToDTO(folder))
	}
}

func moveFolder(d Deps) http.HandlerFunc {
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
		var in dto.MoveFolderRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
			return
		}
		var newParentID *uuid.UUID
		if in.ParentID != nil {
			parsed, err := uuid.Parse(*in.ParentID)
			if err != nil {
				restapi.WriteError(w, http.StatusBadRequest, "invalid parent_id")
				return
			}
			newParentID = &parsed
		}
		if err := d.Storage.MoveFolder(r.Context(), storageuc.MoveFolderParams{
			FolderID:    folderID,
			UserID:      uid,
			NewParentID: newParentID,
		}); err != nil {
			writeFolderErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func deleteFolder(d Deps) http.HandlerFunc {
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
		if err := d.Storage.DeleteFolder(r.Context(), uid, folderID); err != nil {
			writeFolderErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func moveBlob(d Deps) http.HandlerFunc {
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
		var in dto.MoveBlobRequest
		if err := restapi.DecodeJSON(r, &in); err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "bad request")
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
		if err := d.Storage.MoveBlob(r.Context(), uid, blobID, folderID); err != nil {
			writeFolderErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func folderToDTO(f entity.Folder) dto.FolderItem {
	item := dto.FolderItem{
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

func writeFolderErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storageuc.ErrFolderNotFound):
		restapi.WriteError(w, http.StatusNotFound, "folder not found")
	case errors.Is(err, storageuc.ErrFolderNotEmpty):
		restapi.WriteError(w, http.StatusConflict, "folder is not empty")
	case errors.Is(err, storageuc.ErrFolderConflict):
		restapi.WriteError(w, http.StatusConflict, "folder with this name already exists")
	case errors.Is(err, storageuc.ErrFolderCycle):
		restapi.WriteError(w, http.StatusUnprocessableEntity, "moving folder would create a cycle")
	case errors.Is(err, storageuc.ErrNotFound):
		restapi.WriteError(w, http.StatusNotFound, "not found")
	default:
		restapi.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}
