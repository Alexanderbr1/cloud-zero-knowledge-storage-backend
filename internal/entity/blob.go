package entity

import (
	"time"

	"github.com/google/uuid"
)

type Blob struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	FolderID         *uuid.UUID
	FileName         string
	ContentType      string
	ObjectKey        string
	FileSize         int64
	FileSizePlain    int64 // plaintext size in bytes
	ChunkSize        int32 // frame chunk size used during encryption
	CreatedAt        time.Time
	UploadedAt       *time.Time
	EncryptedFileKey []byte // AES-KW wrapped file key
	UploadID         string // S3 multipart upload ID; empty after CompleteMultipartUpload
	DeletedAt        *time.Time
}
