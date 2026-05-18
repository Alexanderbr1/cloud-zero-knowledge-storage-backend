package v1

import (
	"context"
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

type FolderService interface {
	CreateFolder(ctx context.Context, p storageuc.CreateFolderParams) (entity.Folder, string, error)
	GetFolder(ctx context.Context, userID, folderID uuid.UUID) (entity.Folder, error)
	ListFolders(ctx context.Context, userID uuid.UUID, parentID *uuid.UUID) ([]entity.Folder, error)
	RenameFolder(ctx context.Context, userID, folderID uuid.UUID, name string) (entity.Folder, error)
	MoveFolder(ctx context.Context, p storageuc.MoveFolderParams) (storageuc.FolderMoveInfo, error)
	DeleteFolder(ctx context.Context, userID, folderID uuid.UUID) (string, error)
	MoveBlob(ctx context.Context, userID, blobID uuid.UUID, folderID *uuid.UUID) (storageuc.MoveBlobInfo, error)
}

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
		folder, parentFolderName, err := d.Folders.CreateFolder(r.Context(), storageuc.CreateFolderParams{
			UserID:   uid,
			ParentID: parentID,
			Name:     name,
		})
		if err != nil {
			writeFolderErr(w, err, d.Logger)
			return
		}
		folderID := folder.ID
		resourceName := folder.Name
		if parentFolderName != "" {
			resourceName = folder.Name + " (в папке: " + parentFolderName + ")"
		}
		d.Audit.LogAsync(r.Context(), auditEvent(uid, entity.AuditFolderCreated, realIP(r), r.Header.Get("User-Agent"), &folderID, resourceName))
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
		folder, err := d.Folders.GetFolder(r.Context(), uid, folderID)
		if err != nil {
			writeFolderErr(w, err, d.Logger)
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
		folders, err := d.Folders.ListFolders(r.Context(), uid, parentID)
		if err != nil {
			writeFolderErr(w, err, d.Logger)
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
		folder, err := d.Folders.RenameFolder(r.Context(), uid, folderID, name)
		if err != nil {
			writeFolderErr(w, err, d.Logger)
			return
		}
		fid := folder.ID
		d.Audit.LogAsync(r.Context(), auditEvent(uid, entity.AuditFolderRenamed, realIP(r), r.Header.Get("User-Agent"), &fid, folder.Name))
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
		info, err := d.Folders.MoveFolder(r.Context(), storageuc.MoveFolderParams{
			FolderID:    folderID,
			UserID:      uid,
			NewParentID: newParentID,
		})
		if err != nil {
			writeFolderErr(w, err, d.Logger)
			return
		}
		src := info.SrcFolderName
		if src == "" {
			src = "/"
		}
		dst := info.DstFolderName
		if dst == "" {
			dst = "/"
		}
		resourceName := info.FolderName + " (" + src + " → " + dst + ")"
		d.Audit.LogAsync(r.Context(), auditEvent(uid, entity.AuditFolderMoved, realIP(r), r.Header.Get("User-Agent"), &folderID, resourceName))
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
		name, err := d.Folders.DeleteFolder(r.Context(), uid, folderID)
		if err != nil {
			writeFolderErr(w, err, d.Logger)
			return
		}
		d.Audit.LogAsync(r.Context(), auditEvent(uid, entity.AuditFolderDeleted, realIP(r), r.Header.Get("User-Agent"), &folderID, name))
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
		info, err := d.Folders.MoveBlob(r.Context(), uid, blobID, folderID)
		if err != nil {
			writeFolderErr(w, err, d.Logger)
			return
		}
		src := info.SrcFolderName
		if src == "" {
			src = "/"
		}
		dst := info.DstFolderName
		if dst == "" {
			dst = "/"
		}
		resourceName := info.FileName + " (" + src + " → " + dst + ")"
		d.Audit.LogAsync(r.Context(), auditEvent(uid, entity.AuditFileMoved, realIP(r), r.Header.Get("User-Agent"), &blobID, resourceName))
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

func writeFolderErr(w http.ResponseWriter, err error, log zerolog.Logger) {
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
		restapi.WriteInternalError(w, log, err)
	}
}
