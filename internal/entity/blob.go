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
	FileSizePlain    int64
	ChunkSize        int32
	CreatedAt        time.Time
	UploadedAt       *time.Time
	EncryptedFileKey []byte
	UploadID         string
	DeletedAt        *time.Time
}
