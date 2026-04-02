package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	storageuc "cloud-backend/internal/usecase/storage"
)

var _ storageuc.BlobRegistry = (*Storage)(nil)

func (s *Storage) RegisterStoredBlob(ctx context.Context, id, userID uuid.UUID, fileName, objectKey, uploadMethod string) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO stored_blobs (id, user_id, file_name, object_key, upload_method)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, userID, fileName, objectKey, uploadMethod,
	)
	return err
}

func (s *Storage) GetBlobForUser(ctx context.Context, blobID, userID uuid.UUID) (objectKey string, ok bool, err error) {
	err = s.Pool.QueryRow(ctx,
		`SELECT object_key FROM stored_blobs WHERE id = $1 AND user_id = $2`,
		blobID, userID,
	).Scan(&objectKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return objectKey, true, nil
}

func (s *Storage) DeleteBlobRow(ctx context.Context, blobID, userID uuid.UUID) (int64, error) {
	tag, err := s.Pool.Exec(ctx,
		`DELETE FROM stored_blobs WHERE id = $1 AND user_id = $2`,
		blobID, userID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Storage) ListBlobsForUser(ctx context.Context, userID uuid.UUID) ([]storageuc.BlobInfo, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, file_name, object_key, created_at
		 FROM stored_blobs
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []storageuc.BlobInfo
	for rows.Next() {
		var item storageuc.BlobInfo
		var fileName string
		var createdAt time.Time
		if err := rows.Scan(&item.BlobID, &fileName, &item.ObjectKey, &createdAt); err != nil {
			return nil, err
		}
		item.FileName = fileName
		item.CreatedAt = createdAt
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
