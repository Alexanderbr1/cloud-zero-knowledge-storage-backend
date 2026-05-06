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

func (s *Storage) RegisterBlob(ctx context.Context, p storageuc.RegisterBlobParams) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO stored_blobs (id, user_id, file_name, content_type, object_key, file_size, encrypted_file_key, file_iv, folder_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		p.ID, p.UserID, p.FileName, p.ContentType, p.ObjectKey, p.FileSize, p.EncryptedFileKey, p.FileIV, p.FolderID,
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
		`SELECT id, folder_id, file_name, content_type, object_key, file_size, created_at, encrypted_file_key, file_iv
		 FROM stored_blobs
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBlobs(rows, userID)
}

func (s *Storage) ListBlobsInFolder(ctx context.Context, userID uuid.UUID, folderID *uuid.UUID) ([]entity.Blob, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if folderID == nil {
		rows, err = s.pool.Query(ctx,
			`SELECT id, folder_id, file_name, content_type, object_key, file_size, created_at, encrypted_file_key, file_iv
			 FROM stored_blobs
			 WHERE user_id = $1 AND folder_id IS NULL
			 ORDER BY created_at DESC`,
			userID,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT id, folder_id, file_name, content_type, object_key, file_size, created_at, encrypted_file_key, file_iv
			 FROM stored_blobs
			 WHERE user_id = $1 AND folder_id = $2
			 ORDER BY created_at DESC`,
			userID, folderID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBlobs(rows, userID)
}

func scanBlobs(rows pgx.Rows, userID uuid.UUID) ([]entity.Blob, error) {
	var out []entity.Blob
	for rows.Next() {
		b := entity.Blob{UserID: userID}
		if err := rows.Scan(&b.ID, &b.FolderID, &b.FileName, &b.ContentType, &b.ObjectKey, &b.FileSize, &b.CreatedAt, &b.EncryptedFileKey, &b.FileIV); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// MoveBlob updates the folder_id of a blob. Returns false if the blob was not found.
func (s *Storage) MoveBlob(ctx context.Context, blobID, userID uuid.UUID, folderID *uuid.UUID) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE stored_blobs SET folder_id = $3 WHERE id = $1 AND user_id = $2`,
		blobID, userID, folderID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// RenameBlob updates the file_name of a blob. Returns false if the blob was not found.
func (s *Storage) RenameBlob(ctx context.Context, blobID, userID uuid.UUID, name string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE stored_blobs SET file_name = $3 WHERE id = $1 AND user_id = $2`,
		blobID, userID, name,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Storage) SearchBlobs(ctx context.Context, userID uuid.UUID, query string) ([]storageuc.SearchBlobRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT b.id, b.folder_id, b.file_name, b.content_type, b.object_key, b.file_size,
		        b.created_at, b.encrypted_file_key, b.file_iv, f.name AS folder_name
		 FROM stored_blobs b
		 LEFT JOIN folders f ON f.id = b.folder_id
		 WHERE b.user_id = $1 AND b.file_name ILIKE $2
		 ORDER BY b.created_at DESC
		 LIMIT 200`,
		userID, "%"+query+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSearchBlobs(rows, userID)
}

func scanSearchBlobs(rows pgx.Rows, userID uuid.UUID) ([]storageuc.SearchBlobRecord, error) {
	var out []storageuc.SearchBlobRecord
	for rows.Next() {
		var rec storageuc.SearchBlobRecord
		rec.UserID = userID
		if err := rows.Scan(
			&rec.ID, &rec.FolderID, &rec.FileName, &rec.ContentType, &rec.ObjectKey,
			&rec.FileSize, &rec.CreatedAt, &rec.EncryptedFileKey, &rec.FileIV, &rec.FolderName,
		); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
