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
	EncryptedFileKey []byte
	FileIV           []byte
	FolderID         *uuid.UUID
}

type BlobMeta struct {
	ObjectKey        string
	ContentType      string
	EncryptedFileKey []byte
	FileIV           []byte
	FileName         string
}

type PendingBlobMeta struct {
	ObjectKey    string
	DeclaredSize int64
	FileName     string
	FolderID     *uuid.UUID
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

type PresignPutParams struct {
	UserID           uuid.UUID
	FileName         string
	ContentType      string
	FileSize         int64
	EncryptedFileKey []byte
	FileIV           []byte
	FolderID         *uuid.UUID
}

type PresignPutResult struct {
	BlobID      uuid.UUID
	ObjectKey   string
	UploadURL   string
	ExpiresIn   int64
	HTTPMethod  string
	ContentType string
}

type PresignGetResult struct {
	BlobID           uuid.UUID
	ObjectKey        string
	DownloadURL      string
	ExpiresIn        int64
	HTTPMethod       string
	ContentType      string
	EncryptedFileKey []byte
	FileIV           []byte
	FileName         string
}

type SearchParams struct {
	UserID uuid.UUID
	Query  string
}

type SearchResult struct {
	Blobs   []SearchBlobRecord
	Folders []entity.Folder
}
