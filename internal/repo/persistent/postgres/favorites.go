package postgres

import (
	"context"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
	favoritesuc "cloud-backend/internal/usecase/favorites"
)

var _ favoritesuc.FavoritesRepo = (*Storage)(nil)

func (s *Storage) AddBlobFavorite(ctx context.Context, userID, blobID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO favorites (user_id, blob_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, blobID,
	)
	return err
}

func (s *Storage) RemoveBlobFavorite(ctx context.Context, userID, blobID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM favorites WHERE user_id = $1 AND blob_id = $2`,
		userID, blobID,
	)
	return err
}

func (s *Storage) AddFolderFavorite(ctx context.Context, userID, folderID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO favorites (user_id, folder_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, folderID,
	)
	return err
}

func (s *Storage) RemoveFolderFavorite(ctx context.Context, userID, folderID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM favorites WHERE user_id = $1 AND folder_id = $2`,
		userID, folderID,
	)
	return err
}

func (s *Storage) ListFavoriteBlobs(ctx context.Context, userID uuid.UUID) ([]favoritesuc.FavoriteBlob, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT b.id, b.file_name, b.content_type, b.object_key, b.file_size,
		        b.created_at, b.encrypted_file_key, b.chunk_size, b.file_size_plain, b.folder_id,
		        f.name AS folder_name
		 FROM stored_blobs b
		 JOIN favorites fav ON fav.blob_id = b.id AND fav.user_id = $1
		 LEFT JOIN folders f ON f.id = b.folder_id AND f.deleted_at IS NULL
		 WHERE b.user_id = $1 AND b.deleted_at IS NULL AND b.uploaded_at IS NOT NULL
		 ORDER BY fav.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blobs []favoritesuc.FavoriteBlob
	for rows.Next() {
		var fb favoritesuc.FavoriteBlob
		if err := rows.Scan(
			&fb.ID, &fb.FileName, &fb.ContentType, &fb.ObjectKey, &fb.FileSize,
			&fb.CreatedAt, &fb.EncryptedFileKey, &fb.ChunkSize, &fb.FileSizePlain, &fb.FolderID,
			&fb.FolderName,
		); err != nil {
			return nil, err
		}
		blobs = append(blobs, fb)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if blobs == nil {
		return []favoritesuc.FavoriteBlob{}, nil
	}
	return blobs, nil
}

func (s *Storage) ListFavoriteFolders(ctx context.Context, userID uuid.UUID) ([]entity.Folder, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT fo.id, fo.name, fo.parent_id, fo.created_at
		 FROM folders fo
		 JOIN favorites fav ON fav.folder_id = fo.id AND fav.user_id = $1
		 WHERE fo.user_id = $1 AND fo.deleted_at IS NULL
		 ORDER BY fav.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []entity.Folder
	for rows.Next() {
		var f entity.Folder
		if err := rows.Scan(&f.ID, &f.Name, &f.ParentID, &f.CreatedAt); err != nil {
			return nil, err
		}
		folders = append(folders, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if folders == nil {
		return []entity.Folder{}, nil
	}
	return folders, nil
}
