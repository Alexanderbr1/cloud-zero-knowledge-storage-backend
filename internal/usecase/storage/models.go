package storage

import (
	"github.com/google/uuid"

	"cloud-backend/internal/entity"
)

type TrashResult struct {
	Blobs   []entity.Blob
	Folders []entity.Folder
}

type EmptiedBlob struct {
	ID        uuid.UUID
	FileName  string
	ObjectKey string
}

type EmptiedFolder struct {
	ID   uuid.UUID
	Name string
}

type RegisterBlobParams struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	FileName         string
	ContentType      string
	ObjectKey        string
	FileSize         int64
	FileSizePlain    int64
	ChunkSize        int32
	EncryptedFileKey []byte
	FolderID         *uuid.UUID
	UploadID         string // non-empty for multipart uploads
}

// ─── Multipart ────────────────────────────────────────────────────────────────

type InitiateMultipartParams struct {
	UserID           uuid.UUID
	FileName         string
	ContentType      string
	FileSize         int64
	FileSizePlain    int64
	ChunkSize        int32
	PartCount        int
	EncryptedFileKey []byte
	FolderID         *uuid.UUID
}

type PartURL struct {
	PartNumber int
	URL        string
}

type InitiateMultipartResult struct {
	BlobID   uuid.UUID
	UploadID string
	PartURLs []PartURL
}

type CompleteMultipartParams struct {
	BlobID uuid.UUID
	UserID uuid.UUID
}

type OrphanedBlob struct {
	ObjectKey string
	UploadID  string // empty for single-PUT blobs
}

type BlobMeta struct {
	ObjectKey        string
	ContentType      string
	EncryptedFileKey []byte
	FileName         string
	FileSize         int64
	FileSizePlain    int64
	ChunkSize        int32
}

type PendingBlobMeta struct {
	ObjectKey    string
	DeclaredSize int64
	FileName     string
	FolderID     *uuid.UUID
	UploadID     string
}

type SearchBlobRecord struct {
	entity.Blob
	FolderName *string
}

type CreateFolderParams struct {
	UserID   uuid.UUID
	ParentID *uuid.UUID
	Name     string
}

type MoveBlobInfo struct {
	FileName      string
	SrcFolderName string // empty string means root
	DstFolderName string // empty string means root
}

type FolderMoveInfo struct {
	FolderName    string
	SrcFolderName string // empty string means root
	DstFolderName string // empty string means root
}

type MoveFolderParams struct {
	FolderID    uuid.UUID
	UserID      uuid.UUID
	NewParentID *uuid.UUID
}

type PresignGetResult struct {
	BlobID           uuid.UUID
	ObjectKey        string
	DownloadURL      string
	ExpiresIn        int64
	HTTPMethod       string
	ContentType      string
	EncryptedFileKey []byte
	FileName         string
	FileSize         int64
	FileSizePlain    int64
	ChunkSize        int32
}

type SearchParams struct {
	UserID uuid.UUID
	Query  string
}

type SearchResult struct {
	Blobs   []SearchBlobRecord
	Folders []entity.Folder
}
