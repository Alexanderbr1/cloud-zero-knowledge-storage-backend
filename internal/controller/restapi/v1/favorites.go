package v1

import (
	"context"
	"encoding/base64"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	"cloud-backend/internal/entity"
	favoritesuc "cloud-backend/internal/usecase/favorites"
)

type FavoritesService interface {
	AddBlob(ctx context.Context, userID, blobID uuid.UUID) error
	RemoveBlob(ctx context.Context, userID, blobID uuid.UUID) error
	AddFolder(ctx context.Context, userID, folderID uuid.UUID) error
	RemoveFolder(ctx context.Context, userID, folderID uuid.UUID) error
	ListBlobs(ctx context.Context, userID uuid.UUID) ([]favoritesuc.FavoriteBlob, error)
	ListFolders(ctx context.Context, userID uuid.UUID) ([]entity.Folder, error)
}

func favoritesAdd(d Deps, resourceType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid id")
			return
		}
		switch resourceType {
		case "blob":
			err = d.Favorites.AddBlob(r.Context(), uid, id)
		case "folder":
			err = d.Favorites.AddFolder(r.Context(), uid, id)
		}
		if err != nil {
			restapi.WriteInternalError(w, d.Logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func favoritesRemove(d Deps, resourceType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid id")
			return
		}
		switch resourceType {
		case "blob":
			err = d.Favorites.RemoveBlob(r.Context(), uid, id)
		case "folder":
			err = d.Favorites.RemoveFolder(r.Context(), uid, id)
		}
		if err != nil {
			restapi.WriteInternalError(w, d.Logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func favoritesList(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}

		blobs, err := d.Favorites.ListBlobs(r.Context(), uid)
		if err != nil {
			restapi.WriteInternalError(w, d.Logger, err)
			return
		}
		folders, err := d.Favorites.ListFolders(r.Context(), uid)
		if err != nil {
			restapi.WriteInternalError(w, d.Logger, err)
			return
		}

		blobItems := make([]dto.SearchBlobItem, 0, len(blobs))
		for _, b := range blobs {
			blobItems = append(blobItems, favoriteBlobToDTO(b))
		}
		folderItems := make([]dto.FolderItem, 0, len(folders))
		for _, f := range folders {
			folderItems = append(folderItems, folderToDTO(f))
		}

		restapi.WriteJSON(w, http.StatusOK, dto.FavoritesResponse{
			Blobs:   blobItems,
			Folders: folderItems,
		})
	}
}

func favoriteBlobToDTO(b favoritesuc.FavoriteBlob) dto.SearchBlobItem {
	item := dto.SearchBlobItem{
		BlobID:           b.ID.String(),
		FileName:         b.FileName,
		ContentType:      b.ContentType,
		FileSize:         b.FileSize,
		FileSizePlain:    b.FileSizePlain,
		ChunkSize:        b.ChunkSize,
		CreatedAt:        b.CreatedAt,
		EncryptedFileKey: base64.StdEncoding.EncodeToString(b.EncryptedFileKey),
		FolderName:       b.FolderName,
	}
	if b.FolderID != nil {
		s := b.FolderID.String()
		item.FolderID = &s
	}
	return item
}
