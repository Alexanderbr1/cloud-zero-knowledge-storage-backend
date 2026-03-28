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
	RegisterStoredBlob(ctx context.Context, id uuid.UUID, fileName, objectKey, contentType, uploadMethod string) error
	GetBlob(ctx context.Context, blobID uuid.UUID) (objectKey, contentType string, ok bool, err error)
	DeleteBlobRow(ctx context.Context, blobID uuid.UUID) (rowsAffected int64, err error)
	ListBlobs(ctx context.Context) ([]BlobInfo, error)
}

// contentType — уже нормализованный слоем доставки (непустой, длина проверена в DTO).
func (s *Service) PresignPut(ctx context.Context, contentType, fileName string) (*PresignPutResult, error) {
	blobID := uuid.New()
	cleanName := sanitizeFileName(fileName)
	objectKey := fmt.Sprintf("%s/%s", blobID.String(), cleanName)

	if err := s.Blobs.RegisterStoredBlob(ctx, blobID, cleanName, objectKey, contentType, methodPresign); err != nil {
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

// PresignGet выдаёт подписанный GET для скачивания файла.
func (s *Service) PresignGet(ctx context.Context, blobID uuid.UUID) (*PresignGetResult, error) {
	objectKey, contentType, ok, err := s.Blobs.GetBlob(ctx, blobID)
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

// DeleteBlob удаляет объект в хранилище, затем строку в БД (чтобы не терять ссылку на ключ при сбое MinIO).
func (s *Service) DeleteBlob(ctx context.Context, blobID uuid.UUID) error {
	objectKey, _, ok, err := s.Blobs.GetBlob(ctx, blobID)
	if err != nil {
		return fmt.Errorf("get blob: %w", err)
	}
	if !ok {
		return ErrNotFound
	}
	if err := s.Objects.RemoveObject(ctx, objectKey); err != nil {
		return fmt.Errorf("remove object: %w", err)
	}
	// n == 0 означает параллельное удаление: объект уже убран — считаем успехом.
	if _, err := s.Blobs.DeleteBlobRow(ctx, blobID); err != nil {
		return fmt.Errorf("delete blob row: %w", err)
	}
	return nil
}

func (s *Service) ListBlobs(ctx context.Context) ([]BlobInfo, error) {
	items, err := s.Blobs.ListBlobs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list blobs: %w", err)
	}
	return items, nil
}
