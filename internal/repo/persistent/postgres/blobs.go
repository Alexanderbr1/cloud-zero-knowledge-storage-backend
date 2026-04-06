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

func (s *Storage) RegisterBlob(
	ctx context.Context,
	id, userID uuid.UUID,
	fileName, contentType, objectKey, uploadMethod string,
	encryptedFileKey, fileIV []byte,
) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO stored_blobs (id, user_id, file_name, content_type, object_key, upload_method, encrypted_file_key, file_iv)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, userID, fileName, contentType, objectKey, uploadMethod, encryptedFileKey, fileIV,
	)
	return err
}

func (s *Storage) GetBlobMeta(ctx context.Context, blobID, userID uuid.UUID) (storageuc.BlobMeta, bool, error) {
	var m storageuc.BlobMeta
	err := s.pool.QueryRow(ctx,
		`SELECT object_key, content_type, encrypted_file_key, file_iv
		 FROM stored_blobs WHERE id = $1 AND user_id = $2`,
		blobID, userID,
	).Scan(&m.ObjectKey, &m.ContentType, &m.EncryptedFileKey, &m.FileIV)
	if errors.Is(err, pgx.ErrNoRows) {
		return storageuc.BlobMeta{}, false, nil
	}
	if err != nil {
		return storageuc.BlobMeta{}, false, err
	}
	return m, true, nil
}

// RemoveBlob атомарно удаляет запись и возвращает objectKey для последующего удаления из MinIO.
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
		`SELECT id, file_name, content_type, object_key, created_at, encrypted_file_key, file_iv
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
		if err := rows.Scan(&b.ID, &b.FileName, &b.ContentType, &b.ObjectKey, &b.CreatedAt, &b.EncryptedFileKey, &b.FileIV); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
