package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cloud-backend/internal/entity"
	sharinguc "cloud-backend/internal/usecase/sharing"
)

var _ sharinguc.ShareRepository = (*Storage)(nil)
var _ sharinguc.UserKeyStore = (*Storage)(nil)
var _ sharinguc.BlobStore = (*Storage)(nil)

// ─── UserKeyStore ─────────────────────────────────────────────────────────

func (s *Storage) GetPublicKeyByEmail(ctx context.Context, email string) ([]byte, uuid.UUID, error) {
	var id uuid.UUID
	var pub []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, public_key FROM users WHERE email = $1`,
		email,
	).Scan(&id, &pub)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, uuid.Nil, sharinguc.ErrNotFound
	}
	if err != nil {
		return nil, uuid.Nil, err
	}
	if len(pub) == 0 {
		return nil, uuid.Nil, sharinguc.ErrNoPublicKey
	}
	return pub, id, nil
}

// ─── BlobStore ────────────────────────────────────────────────────────────

func (s *Storage) GetBlobInfo(ctx context.Context, blobID uuid.UUID) (sharinguc.BlobInfo, bool, error) {
	var info sharinguc.BlobInfo
	err := s.pool.QueryRow(ctx,
		`SELECT object_key, file_name, content_type, file_iv, user_id
		 FROM stored_blobs WHERE id = $1`,
		blobID,
	).Scan(&info.ObjectKey, &info.FileName, &info.ContentType, &info.FileIV, &info.OwnerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return sharinguc.BlobInfo{}, false, nil
	}
	if err != nil {
		return sharinguc.BlobInfo{}, false, err
	}
	return info, true, nil
}

// ─── ShareRepository ──────────────────────────────────────────────────────

func (s *Storage) CreateShare(ctx context.Context, p sharinguc.CreateShareParams) (entity.FileShare, error) {
	var share entity.FileShare
	err := s.pool.QueryRow(ctx,
		`WITH inserted AS (
		     INSERT INTO file_shares (blob_id, owner_id, recipient_id, ephemeral_pub, wrapped_file_key, expires_at)
		     VALUES ($1, $2, $3, $4, $5, $6)
		     RETURNING id, blob_id, owner_id, recipient_id, ephemeral_pub, wrapped_file_key, expires_at, revoked_at, created_at
		 )
		 SELECT i.id, i.blob_id, i.owner_id, i.recipient_id,
		        i.ephemeral_pub, i.wrapped_file_key, i.expires_at, i.revoked_at, i.created_at,
		        sb.file_name, sb.content_type, owner.email, recipient.email
		 FROM inserted i
		 JOIN stored_blobs sb ON sb.id = i.blob_id
		 JOIN users owner     ON owner.id = i.owner_id
		 JOIN users recipient ON recipient.id = i.recipient_id`,
		p.BlobID, p.OwnerID, p.RecipientID, p.EphemeralPub, p.WrappedFileKey, p.ExpiresAt,
	).Scan(
		&share.ID, &share.BlobID, &share.OwnerID, &share.RecipientID,
		&share.EphemeralPub, &share.WrappedFileKey, &share.ExpiresAt, &share.RevokedAt, &share.CreatedAt,
		&share.BlobFileName, &share.BlobContentType, &share.OwnerEmail, &share.RecipientEmail,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return entity.FileShare{}, sharinguc.ErrDuplicateShare
		}
		return entity.FileShare{}, err
	}
	return share, nil
}

func (s *Storage) GetShare(ctx context.Context, shareID uuid.UUID) (entity.FileShare, bool, error) {
	var share entity.FileShare
	err := s.pool.QueryRow(ctx,
		`SELECT fs.id, fs.blob_id, fs.owner_id, fs.recipient_id,
		        fs.ephemeral_pub, fs.wrapped_file_key, fs.expires_at, fs.revoked_at, fs.created_at,
		        sb.file_name, sb.content_type, owner.email, recipient.email
		 FROM file_shares fs
		 JOIN stored_blobs sb ON sb.id = fs.blob_id
		 JOIN users owner     ON owner.id = fs.owner_id
		 JOIN users recipient ON recipient.id = fs.recipient_id
		 WHERE fs.id = $1`,
		shareID,
	).Scan(
		&share.ID, &share.BlobID, &share.OwnerID, &share.RecipientID,
		&share.EphemeralPub, &share.WrappedFileKey, &share.ExpiresAt, &share.RevokedAt, &share.CreatedAt,
		&share.BlobFileName, &share.BlobContentType, &share.OwnerEmail, &share.RecipientEmail,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return entity.FileShare{}, false, nil
	}
	if err != nil {
		return entity.FileShare{}, false, err
	}
	return share, true, nil
}

func (s *Storage) ListSharedWithUser(ctx context.Context, recipientID uuid.UUID) ([]entity.FileShare, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT fs.id, fs.blob_id, fs.owner_id, fs.recipient_id,
		        fs.ephemeral_pub, fs.wrapped_file_key, fs.expires_at, fs.revoked_at, fs.created_at,
		        sb.file_name, sb.content_type, owner.email, recipient.email
		 FROM file_shares fs
		 JOIN stored_blobs sb ON sb.id = fs.blob_id
		 JOIN users owner     ON owner.id = fs.owner_id
		 JOIN users recipient ON recipient.id = fs.recipient_id
		 WHERE fs.recipient_id = $1
		   AND fs.revoked_at IS NULL
		   AND (fs.expires_at IS NULL OR fs.expires_at > now())
		 ORDER BY fs.created_at DESC`,
		recipientID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []entity.FileShare
	for rows.Next() {
		var fs entity.FileShare
		if err := rows.Scan(
			&fs.ID, &fs.BlobID, &fs.OwnerID, &fs.RecipientID,
			&fs.EphemeralPub, &fs.WrappedFileKey, &fs.ExpiresAt, &fs.RevokedAt, &fs.CreatedAt,
			&fs.BlobFileName, &fs.BlobContentType, &fs.OwnerEmail, &fs.RecipientEmail,
		); err != nil {
			return nil, err
		}
		out = append(out, fs)
	}
	return out, rows.Err()
}

func (s *Storage) ListSharesForBlob(ctx context.Context, blobID, ownerID uuid.UUID) ([]entity.FileShare, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT fs.id, fs.blob_id, fs.owner_id, fs.recipient_id,
		        fs.ephemeral_pub, fs.wrapped_file_key, fs.expires_at, fs.revoked_at, fs.created_at,
		        sb.file_name, sb.content_type, owner.email, recipient.email
		 FROM file_shares fs
		 JOIN stored_blobs sb ON sb.id = fs.blob_id
		 JOIN users owner     ON owner.id = fs.owner_id
		 JOIN users recipient ON recipient.id = fs.recipient_id
		 WHERE fs.blob_id = $1 AND fs.owner_id = $2
		   AND fs.revoked_at IS NULL
		 ORDER BY fs.created_at DESC`,
		blobID, ownerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []entity.FileShare
	for rows.Next() {
		var fs entity.FileShare
		if err := rows.Scan(
			&fs.ID, &fs.BlobID, &fs.OwnerID, &fs.RecipientID,
			&fs.EphemeralPub, &fs.WrappedFileKey, &fs.ExpiresAt, &fs.RevokedAt, &fs.CreatedAt,
			&fs.BlobFileName, &fs.BlobContentType, &fs.OwnerEmail, &fs.RecipientEmail,
		); err != nil {
			return nil, err
		}
		out = append(out, fs)
	}
	return out, rows.Err()
}

func (s *Storage) RevokeShare(ctx context.Context, shareID, ownerID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE file_shares SET revoked_at = now()
		 WHERE id = $1 AND owner_id = $2 AND revoked_at IS NULL`,
		shareID, ownerID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return sharinguc.ErrNotFound
	}
	return nil
}
