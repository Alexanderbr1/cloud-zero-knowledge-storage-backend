package minio

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type StoreConfig struct {
	Endpoint       string
	PublicEndpoint string
	AccessKey      string
	SecretKey      string
	Bucket         string
	UseSSL         bool
	Region         string
}

type Store struct {
	client        *minio.Client
	presignClient *minio.Client
	bucket        string
}

func NewStore(cfg StoreConfig) (*Store, error) {
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

func (s *Store) PresignedPutObject(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error) {
	return s.presignClient.PresignedPutObject(ctx, s.bucket, objectKey, expiry)
}

func (s *Store) PresignedGetObject(ctx context.Context, objectKey string, expiry time.Duration) (*url.URL, error) {
	return s.presignClient.PresignedGetObject(ctx, s.bucket, objectKey, expiry, nil)
}

// StatObject returns the actual size of an uploaded object.
// Used in ConfirmUpload to verify the real file size against the declared value.
func (s *Store) StatObject(ctx context.Context, objectKey string) (int64, error) {
	info, err := s.client.StatObject(ctx, s.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return 0, err
	}
	return info.Size, nil
}

func (s *Store) RemoveObject(ctx context.Context, objectKey string) error {
	err := s.client.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{})
	if err == nil {
		return nil
	}
	var resp minio.ErrorResponse
	if errors.As(err, &resp) && (resp.Code == "NoSuchKey" || resp.StatusCode == http.StatusNotFound) {
		return nil
	}
	return err
}

// ─── Multipart upload ─────────────────────────────────────────────────────────

// NewMultipartUpload initiates a multipart upload and returns the S3 uploadID.
func (s *Store) NewMultipartUpload(ctx context.Context, objectKey string) (string, error) {
	core := minio.Core{Client: s.client}
	return core.NewMultipartUpload(ctx, s.bucket, objectKey, minio.PutObjectOptions{})
}

// PresignUploadPart returns a presigned PUT URL for a single multipart part.
func (s *Store) PresignUploadPart(ctx context.Context, objectKey, uploadID string, partNumber int, expiry time.Duration) (*url.URL, error) {
	params := url.Values{}
	params.Set("uploadId", uploadID)
	params.Set("partNumber", strconv.Itoa(partNumber))
	return s.presignClient.Presign(ctx, "PUT", s.bucket, objectKey, expiry, params)
}

// CompleteMultipartUpload finalises the multipart upload by listing uploaded parts
// from MinIO (avoids requiring the client to forward ETags) and assembling the object.
func (s *Store) CompleteMultipartUpload(ctx context.Context, objectKey, uploadID string) error {
	core := minio.Core{Client: s.client}

	// Collect all uploaded parts from MinIO — avoids requiring the client to forward ETags.
	var parts []minio.CompletePart
	marker := 0
	for {
		result, err := core.ListObjectParts(ctx, s.bucket, objectKey, uploadID, marker, 1000)
		if err != nil {
			return fmt.Errorf("list object parts: %w", err)
		}
		for _, p := range result.ObjectParts {
			parts = append(parts, minio.CompletePart{
				PartNumber: p.PartNumber,
				ETag:       p.ETag,
			})
		}
		if !result.IsTruncated {
			break
		}
		marker = result.NextPartNumberMarker
	}

	_, err := core.CompleteMultipartUpload(ctx, s.bucket, objectKey, uploadID, parts, minio.PutObjectOptions{})
	return err
}

// AbortMultipartUpload cancels an in-progress multipart upload and frees MinIO storage.
func (s *Store) AbortMultipartUpload(ctx context.Context, objectKey, uploadID string) error {
	core := minio.Core{Client: s.client}
	return core.AbortMultipartUpload(ctx, s.bucket, objectKey, uploadID)
}
