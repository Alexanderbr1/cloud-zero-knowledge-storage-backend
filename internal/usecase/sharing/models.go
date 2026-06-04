package sharing

import (
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/entity"
)

type BlobInfo struct {
	ObjectKey     string
	FileName      string
	ContentType   string
	FileSize      int64
	FileSizePlain int64
	ChunkSize     int32
	OwnerID       uuid.UUID
}

type CreateShareParams struct {
	BlobID         uuid.UUID
	OwnerID        uuid.UUID
	RecipientEmail string    // input field; resolved to RecipientID before reaching repo
	RecipientID    uuid.UUID // populated by Service.CreateShare before calling Shares.CreateShare
	EphemeralPub   []byte
	WrappedFileKey []byte
	ExpiresAt      *time.Time
}

type SharedFileResult struct {
	Share         entity.FileShareView
	DownloadURL   string
	FileName      string
	ContentType   string
	FileSize      int64
	FileSizePlain int64
	ChunkSize     int32
}
