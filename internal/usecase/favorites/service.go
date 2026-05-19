package favorites

import (
	"context"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
)

const dbTimeout = 10 * time.Second

// FavoriteBlob is a blob enriched with its parent folder name for display.
type FavoriteBlob struct {
	entity.Blob
	FolderName *string
}

type FavoritesRepo interface {
	AddBlobFavorite(ctx context.Context, userID, blobID uuid.UUID) error
	RemoveBlobFavorite(ctx context.Context, userID, blobID uuid.UUID) error
	AddFolderFavorite(ctx context.Context, userID, folderID uuid.UUID) error
	RemoveFolderFavorite(ctx context.Context, userID, folderID uuid.UUID) error
	ListFavoriteBlobs(ctx context.Context, userID uuid.UUID) ([]FavoriteBlob, error)
	ListFavoriteFolders(ctx context.Context, userID uuid.UUID) ([]entity.Folder, error)
}

type Service struct {
	Repo FavoritesRepo
}

func (s *Service) AddBlob(ctx context.Context, userID, blobID uuid.UUID) error {
	dbCtx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()
	return s.Repo.AddBlobFavorite(dbCtx, userID, blobID)
}

func (s *Service) RemoveBlob(ctx context.Context, userID, blobID uuid.UUID) error {
	dbCtx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()
	return s.Repo.RemoveBlobFavorite(dbCtx, userID, blobID)
}

func (s *Service) AddFolder(ctx context.Context, userID, folderID uuid.UUID) error {
	dbCtx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()
	return s.Repo.AddFolderFavorite(dbCtx, userID, folderID)
}

func (s *Service) RemoveFolder(ctx context.Context, userID, folderID uuid.UUID) error {
	dbCtx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()
	return s.Repo.RemoveFolderFavorite(dbCtx, userID, folderID)
}

func (s *Service) ListBlobs(ctx context.Context, userID uuid.UUID) ([]FavoriteBlob, error) {
	dbCtx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()
	return s.Repo.ListFavoriteBlobs(dbCtx, userID)
}

func (s *Service) ListFolders(ctx context.Context, userID uuid.UUID) ([]entity.Folder, error) {
	dbCtx, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()
	return s.Repo.ListFavoriteFolders(dbCtx, userID)
}
