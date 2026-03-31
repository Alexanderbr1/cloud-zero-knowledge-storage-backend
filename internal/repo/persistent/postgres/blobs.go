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

func (s *Storage) RegisterStoredBlob(ctx context.Context, id, userID uuid.UUID, fileName, objectKey, contentType, uploadMethod string) error {
	var ct *string
	if contentType != "" {
		ct = &contentType
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO stored_blobs (id, user_id, file_name, object_key, content_type, upload_method)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, userID, fileName, objectKey, ct, uploadMethod,
	)
	return err
}

func (s *Storage) GetBlobForUser(ctx context.Context, blobID, userID uuid.UUID) (objectKey, contentType string, ok bool, err error) {
	var ct *string
	err = s.Pool.QueryRow(ctx,
		`SELECT object_key, content_type FROM stored_blobs WHERE id = $1 AND user_id = $2`,
		blobID, userID,
	).Scan(&objectKey, &ct)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if ct != nil {
		contentType = *ct
	}
	return objectKey, contentType, true, nil
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
		`SELECT id, file_name, object_key, content_type, created_at
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
		var ct *string
		var createdAt time.Time
		if err := rows.Scan(&item.BlobID, &fileName, &item.ObjectKey, &ct, &createdAt); err != nil {
			return nil, err
		}
		item.FileName = fileName
		if ct != nil {
			item.ContentType = *ct
		}
		item.CreatedAt = createdAt
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}
