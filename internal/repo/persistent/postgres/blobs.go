package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cloud-backend/internal/entity"
	storageuc "cloud-backend/internal/usecase/storage"
)

var _ storageuc.BlobRegistry = (*Storage)(nil)

func (s *Storage) RegisterBlob(ctx context.Context, id, userID uuid.UUID, fileName, objectKey, uploadMethod string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO stored_blobs (id, user_id, file_name, object_key, upload_method)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, userID, fileName, objectKey, uploadMethod,
	)
	return err
}

func (s *Storage) GetBlobObjectKey(ctx context.Context, blobID, userID uuid.UUID) (objectKey string, ok bool, err error) {
	err = s.pool.QueryRow(ctx,
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

// RemoveBlob атомарно удаляет запись и возвращает objectKey — без отдельного SELECT.
func (s *Storage) RemoveBlob(ctx context.Context, blobID, userID uuid.UUID) (objectKey string, ok bool, err error) {
	err = s.pool.QueryRow(ctx,
		`DELETE FROM stored_blobs WHERE id = $1 AND user_id = $2 RETURNING object_key`,
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

func (s *Storage) ListBlobs(ctx context.Context, userID uuid.UUID) ([]entity.Blob, error) {
	rows, err := s.pool.Query(ctx,
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

	var out []entity.Blob
	for rows.Next() {
		b := entity.Blob{UserID: userID}
		if err := rows.Scan(&b.ID, &b.FileName, &b.ObjectKey, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
