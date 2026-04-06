package storage

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
)

const methodPresign = "presigned_put"

// Service — загрузка файлов в объектное хранилище.
type Service struct {
	Objects ObjectStore
	Blobs   BlobRegistry

	PresignTTL time.Duration
}

// PresignPutResult — клиент выполняет PUT по UploadURL (тело = файл).
type PresignPutResult struct {
	BlobID      uuid.UUID
	ObjectKey   string
	UploadURL   string
	ExpiresIn   int64
	HTTPMethod  string
	ContentType string
}

// PresignGetResult — клиент выполняет GET по DownloadURL (тело = файл).
type PresignGetResult struct {
	BlobID           uuid.UUID
	ObjectKey        string
	DownloadURL      string
	ExpiresIn        int64
	HTTPMethod       string
	ContentType      string
	EncryptedFileKey []byte
	FileIV           []byte
}

// BlobMeta — объектный ключ и крипто-поля blob'а.
type BlobMeta struct {
	ObjectKey        string
	ContentType      string
	EncryptedFileKey []byte
	FileIV           []byte
}

// ObjectStore — S3-совместимое объектное хранилище.
type ObjectStore interface {
	EnsureBucket(ctx context.Context) error
	PresignedPutObject(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error)
	PresignedGetObject(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error)
	RemoveObject(ctx context.Context, objectKey string) error
}

// BlobRegistry — метаданные blob'ов в БД.
type BlobRegistry interface {
	RegisterBlob(ctx context.Context, id, userID uuid.UUID, fileName, contentType, objectKey, uploadMethod string, encryptedFileKey, fileIV []byte) error
	GetBlobMeta(ctx context.Context, blobID, userID uuid.UUID) (BlobMeta, bool, error)
	// RemoveBlob атомарно удаляет запись и возвращает objectKey для последующего удаления из MinIO.
	RemoveBlob(ctx context.Context, blobID, userID uuid.UUID) (objectKey string, ok bool, err error)
	ListBlobs(ctx context.Context, userID uuid.UUID) ([]entity.Blob, error)
}

// PresignPut создаёт запись и presigned PUT URL; ключ в бакете: blobs/<user_id>/<blob_id>.
func (s *Service) PresignPut(ctx context.Context, userID uuid.UUID, fileName, contentType string, encryptedFileKey, fileIV []byte) (*PresignPutResult, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("presign put: empty user")
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return nil, fmt.Errorf("presign put: empty content_type")
	}
	blobID := uuid.New()
	cleanName := sanitizeFileName(fileName)
	objectKey := fmt.Sprintf("blobs/%s/%s", userID, blobID)

	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	if err := s.Blobs.RegisterBlob(dbCtx, blobID, userID, cleanName, contentType, objectKey, methodPresign, encryptedFileKey, fileIV); err != nil {
		return nil, fmt.Errorf("register blob: %w", err)
	}

	u, err := s.Objects.PresignedPutObject(ctx, objectKey, s.PresignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign put: %w", err)
	}

	return &PresignPutResult{
		BlobID:      blobID,
		ObjectKey:   objectKey,
		UploadURL:   u.String(),
		ExpiresIn:   int64(s.PresignTTL.Seconds()),
		HTTPMethod:  "PUT",
		ContentType: contentType,
	}, nil
}

func (s *Service) PresignGet(ctx context.Context, userID, blobID uuid.UUID) (*PresignGetResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	meta, ok, err := s.Blobs.GetBlobMeta(dbCtx, blobID, userID)
	if err != nil {
		return nil, fmt.Errorf("get blob: %w", err)
	}
	if !ok {
		return nil, ErrNotFound
	}

	u, err := s.Objects.PresignedGetObject(ctx, meta.ObjectKey, s.PresignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign get: %w", err)
	}

	return &PresignGetResult{
		BlobID:           blobID,
		ObjectKey:        meta.ObjectKey,
		DownloadURL:      u.String(),
		ExpiresIn:        int64(s.PresignTTL.Seconds()),
		HTTPMethod:       "GET",
		ContentType:      meta.ContentType,
		EncryptedFileKey: meta.EncryptedFileKey,
		FileIV:           meta.FileIV,
	}, nil
}

// DeleteBlob атомарно удаляет запись в БД (DELETE RETURNING), затем объект из MinIO.
// Если запись отсутствует — возвращает ErrNotFound без обращения к MinIO.
// Если объект в MinIO уже отсутствует — операция идемпотентна.
func (s *Service) DeleteBlob(ctx context.Context, userID, blobID uuid.UUID) error {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	objectKey, ok, err := s.Blobs.RemoveBlob(dbCtx, blobID, userID)
	if err != nil {
		return fmt.Errorf("remove blob record: %w", err)
	}
	if !ok {
		return ErrNotFound
	}

	if err := s.Objects.RemoveObject(ctx, objectKey); err != nil {
		return fmt.Errorf("remove object: %w", err)
	}
	return nil
}

func (s *Service) ListBlobs(ctx context.Context, userID uuid.UUID) ([]entity.Blob, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, 10*time.Second)
	defer dbCancel()

	blobs, err := s.Blobs.ListBlobs(dbCtx, userID)
	if err != nil {
		return nil, fmt.Errorf("list blobs: %w", err)
	}
	return blobs, nil
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "file.bin"
	}
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if name == "." || name == ".." || name == "" {
		return "file.bin"
	}
	return name
}
