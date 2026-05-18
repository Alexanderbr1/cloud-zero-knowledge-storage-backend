package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cloud-backend/internal/entity"
	storageuc "cloud-backend/internal/usecase/storage"
)

var _ storageuc.BlobRepo = (*Storage)(nil)
var _ storageuc.BlobTrashRepo = (*Storage)(nil)

func (s *Storage) GetUserStorageUsed(ctx context.Context, userID uuid.UUID) (int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(file_size), 0) FROM stored_blobs WHERE user_id = $1`,
		userID,
	).Scan(&total)
	return total, err
}

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
		 FROM stored_blobs WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL AND uploaded_at IS NOT NULL`,
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

// RemoveBlob hard-deletes a blob record and returns its objectKey for MinIO cleanup.
// Used internally (e.g. rollback after failed upload). For user-initiated deletes use TrashBlob.
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
		 WHERE user_id = $1 AND deleted_at IS NULL AND uploaded_at IS NOT NULL
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
			 WHERE user_id = $1 AND folder_id IS NULL AND deleted_at IS NULL AND uploaded_at IS NOT NULL
			 ORDER BY created_at DESC`,
			userID,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT id, folder_id, file_name, content_type, object_key, file_size, created_at, encrypted_file_key, file_iv
			 FROM stored_blobs
			 WHERE user_id = $1 AND folder_id = $2 AND deleted_at IS NULL AND uploaded_at IS NOT NULL
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

func (s *Storage) MoveBlob(ctx context.Context, blobID, userID uuid.UUID, folderID *uuid.UUID) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE stored_blobs SET folder_id = $3 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		blobID, userID, folderID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Storage) RenameBlob(ctx context.Context, blobID, userID uuid.UUID, name string) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE stored_blobs SET file_name = $3 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
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
		 WHERE b.user_id = $1 AND b.file_name ILIKE $2 ESCAPE '\' AND b.deleted_at IS NULL AND b.uploaded_at IS NOT NULL
		 ORDER BY b.created_at DESC
		 LIMIT 200`,
		userID, "%"+escapeLike(query)+"%",
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

func (s *Storage) GetPendingBlobMeta(ctx context.Context, blobID, userID uuid.UUID) (storageuc.PendingBlobMeta, bool, error) {
	var m storageuc.PendingBlobMeta
	err := s.pool.QueryRow(ctx,
		`SELECT object_key, file_size FROM stored_blobs
		 WHERE id = $1 AND user_id = $2 AND uploaded_at IS NULL AND deleted_at IS NULL`,
		blobID, userID,
	).Scan(&m.ObjectKey, &m.DeclaredSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return storageuc.PendingBlobMeta{}, false, nil
	}
	if err != nil {
		return storageuc.PendingBlobMeta{}, false, err
	}
	return m, true, nil
}

func (s *Storage) ConfirmBlobUploadWithSize(ctx context.Context, blobID, userID uuid.UUID, actualSize int64) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE stored_blobs SET uploaded_at = NOW(), file_size = $3
		 WHERE id = $1 AND user_id = $2 AND uploaded_at IS NULL AND deleted_at IS NULL`,
		blobID, userID, actualSize,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Storage) PurgeOrphanedBlobs(ctx context.Context, before time.Time) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`DELETE FROM stored_blobs
		 WHERE uploaded_at IS NULL AND deleted_at IS NULL AND created_at < $1
		 RETURNING object_key`,
		before,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObjectKeys(rows)
}

func (s *Storage) TrashBlob(ctx context.Context, blobID, userID uuid.UUID) (bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE stored_blobs
		 SET deleted_at = NOW()
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		blobID, userID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Storage) RestoreBlob(ctx context.Context, blobID, userID uuid.UUID) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE stored_blobs b
		SET deleted_at = NULL,
		    folder_id  = CASE
		        WHEN EXISTS(SELECT 1 FROM folders f WHERE f.id = b.folder_id AND f.deleted_at IS NULL)
		            THEN b.folder_id
		        ELSE NULL
		    END
		WHERE b.id = $1 AND b.user_id = $2 AND b.deleted_at IS NOT NULL`,
		blobID, userID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Storage) HardDeleteBlob(ctx context.Context, blobID, userID uuid.UUID) (objectKey string, ok bool, err error) {
	err = s.pool.QueryRow(ctx,
		`DELETE FROM stored_blobs
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NOT NULL
		 RETURNING object_key`,
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

func (s *Storage) ListTrashedBlobs(ctx context.Context, userID uuid.UUID) ([]entity.Blob, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, folder_id, file_name, content_type, object_key, file_size,
		        created_at, encrypted_file_key, file_iv
		 FROM stored_blobs b
		 WHERE b.user_id = $1
		   AND b.deleted_at IS NOT NULL
		   AND (
		       b.folder_id IS NULL
		       OR NOT EXISTS (
		           SELECT 1 FROM folders f
		           WHERE f.id = b.folder_id AND f.deleted_at IS NOT NULL
		       )
		   )
		 ORDER BY b.deleted_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBlobs(rows, userID)
}

func (s *Storage) EmptyTrashedBlobs(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`DELETE FROM stored_blobs
		 WHERE user_id = $1 AND deleted_at IS NOT NULL
		 RETURNING object_key`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObjectKeys(rows)
}

func (s *Storage) PurgeExpiredBlobs(ctx context.Context, before time.Time) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`DELETE FROM stored_blobs
		 WHERE deleted_at IS NOT NULL AND deleted_at < $1
		 RETURNING object_key`,
		before,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObjectKeys(rows)
}

func scanObjectKeys(rows pgx.Rows) ([]string, error) {
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
