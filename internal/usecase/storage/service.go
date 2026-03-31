package storage

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const methodPresign = "presigned_put"

// Service — загрузка файлов в объектное хранилище.
type Service struct {
	Objects ObjectStore
	Blobs   BlobRegistry

	PresignTTL time.Duration
}

// PresignPutResult — клиент выполняет PUT по upload_url (тело = файл).
type PresignPutResult struct {
	BlobID     uuid.UUID
	ObjectKey  string
	UploadURL  string
	ExpiresIn  int64
	HTTPMethod string
}

// PresignGetResult — клиент выполняет GET по download_url (тело = файл).
type PresignGetResult struct {
	BlobID       uuid.UUID
	ObjectKey    string
	DownloadURL  string
	ExpiresIn    int64
	HTTPMethod   string
	ContentType  string
	Instructions string
}

type BlobInfo struct {
	BlobID      uuid.UUID
	FileName    string
	ObjectKey   string
	ContentType string
	CreatedAt   time.Time
}

// ObjectStore — объектное хранилище S3-совместимое (потребитель: Service).
type ObjectStore interface {
	EnsureBucket(ctx context.Context) error
	PresignedPutObject(ctx context.Context, objectKey string, contentType string, expiry time.Duration) (*url.URL, error)
	PresignedGetObject(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error)
	RemoveObject(ctx context.Context, objectKey string) error
}

// BlobRegistry — метаданные загруженных blob'ов (потребитель: Service).
type BlobRegistry interface {
	RegisterStoredBlob(ctx context.Context, id, userID uuid.UUID, fileName, objectKey, contentType, uploadMethod string) error
	GetBlobForUser(ctx context.Context, blobID, userID uuid.UUID) (objectKey, contentType string, ok bool, err error)
	DeleteBlobRow(ctx context.Context, blobID, userID uuid.UUID) (rowsAffected int64, err error)
	ListBlobsForUser(ctx context.Context, userID uuid.UUID) ([]BlobInfo, error)
}

// PresignPut создаёт запись и presigned PUT; ключ в бакете: blobs/<user_id>/<blob_id>.
func (s *Service) PresignPut(ctx context.Context, userID uuid.UUID, contentType, fileName string) (*PresignPutResult, error) {
	if userID == uuid.Nil {
		return nil, fmt.Errorf("presign put: empty user")
	}
	blobID := uuid.New()
	cleanName := sanitizeFileName(fileName)
	objectKey := fmt.Sprintf("blobs/%s/%s", userID.String(), blobID.String())

	if err := s.Blobs.RegisterStoredBlob(ctx, blobID, userID, cleanName, objectKey, contentType, methodPresign); err != nil {
		return nil, fmt.Errorf("register blob: %w", err)
	}

	u, err := s.Objects.PresignedPutObject(ctx, objectKey, contentType, s.PresignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign put: %w", err)
	}

	return &PresignPutResult{
		BlobID:     blobID,
		ObjectKey:  objectKey,
		UploadURL:  u.String(),
		ExpiresIn:  int64(s.PresignTTL.Seconds()),
		HTTPMethod: "PUT",
	}, nil
}

func sanitizeFileName(fileName string) string {
	name := strings.TrimSpace(fileName)
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

func (s *Service) PresignGet(ctx context.Context, userID, blobID uuid.UUID) (*PresignGetResult, error) {
	objectKey, contentType, ok, err := s.Blobs.GetBlobForUser(ctx, blobID, userID)
	if err != nil {
		return nil, fmt.Errorf("get blob: %w", err)
	}
	if !ok {
		return nil, ErrNotFound
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	u, err := s.Objects.PresignedGetObject(ctx, objectKey, s.PresignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign get: %w", err)
	}

	return &PresignGetResult{
		BlobID:       blobID,
		ObjectKey:    objectKey,
		DownloadURL:  u.String(),
		ExpiresIn:    int64(s.PresignTTL.Seconds()),
		HTTPMethod:   "GET",
		ContentType:  contentType,
		Instructions: "GET download_url to download file bytes",
	}, nil
}

func (s *Service) DeleteBlob(ctx context.Context, userID, blobID uuid.UUID) error {
	objectKey, _, ok, err := s.Blobs.GetBlobForUser(ctx, blobID, userID)
	if err != nil {
		return fmt.Errorf("get blob: %w", err)
	}
	if !ok {
		return ErrNotFound
	}
	if err := s.Objects.RemoveObject(ctx, objectKey); err != nil {
		return fmt.Errorf("remove object: %w", err)
	}
	if _, err := s.Blobs.DeleteBlobRow(ctx, blobID, userID); err != nil {
		return fmt.Errorf("delete blob row: %w", err)
	}
	return nil
}

func (s *Service) ListBlobs(ctx context.Context, userID uuid.UUID) ([]BlobInfo, error) {
	items, err := s.Blobs.ListBlobsForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list blobs: %w", err)
	}
	return items, nil
}
