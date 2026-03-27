package minio

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"cloud-backend/config"
	storageuc "cloud-backend/internal/usecase/storage"
)

// Store — MinIO / S3-совместимый ObjectStore.
type Store struct {
	client        *minio.Client
	presignClient *minio.Client
	bucket        string
}

var _ storageuc.ObjectStore = (*Store)(nil)

func NewStore(cfg config.MinIOConfig) (*Store, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("minio: empty endpoint")
	}
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	}
	cli, err := minio.New(cfg.Endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	presignClient := cli
	if cfg.PublicEndpoint != "" {
		parsed, err := url.Parse(cfg.PublicEndpoint)
		if err != nil {
			return nil, fmt.Errorf("minio public endpoint parse: %w", err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("minio public endpoint must include scheme and host")
		}
		presignClient, err = minio.New(parsed.Host, &minio.Options{
			Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
			Secure: parsed.Scheme == "https",
			Region: cfg.Region,
		})
		if err != nil {
			return nil, fmt.Errorf("minio presign client: %w", err)
		}
	}
	return &Store{
		client:        cli,
		presignClient: presignClient,
		bucket:        cfg.Bucket,
	}, nil
}

func (s *Store) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
}

func (s *Store) PresignedPutObject(ctx context.Context, objectKey string, contentType string, expiry time.Duration) (*url.URL, error) {
	// Для MVP не включаем Content-Type в подписанные заголовки:
	// это упрощает загрузку из браузера и устраняет SignatureDoesNotMatch из-за рассинхрона заголовков.
	return s.presignClient.PresignedPutObject(ctx, s.bucket, objectKey, expiry)
}

func (s *Store) PresignedGetObject(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error) {
	return s.presignClient.PresignedGetObject(ctx, s.bucket, objectKey, expiry, nil)
}

func (s *Store) RemoveObject(ctx context.Context, objectKey string) error {
	err := s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{})
	if err == nil {
		return nil
	}
	// Идемпотентность: объект уже отсутствует (например, не успели залить после presign).
	var resp minio.ErrorResponse
	if errors.As(err, &resp) && (resp.Code == "NoSuchKey" || resp.StatusCode == http.StatusNotFound) {
		return nil
	}
	return err
}
