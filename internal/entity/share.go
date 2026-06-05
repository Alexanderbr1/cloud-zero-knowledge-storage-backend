package entity

import (
	"time"

	"github.com/google/uuid"
)

type FileShare struct {
	ID             uuid.UUID
	BlobID         uuid.UUID
	OwnerID        uuid.UUID
	RecipientID    uuid.UUID
	EphemeralPub   []byte
	WrappedFileKey []byte
	ExpiresAt      *time.Time
	RevokedAt      *time.Time
	CreatedAt      time.Time
}

type FileShareView struct {
	FileShare
	BlobFileName    string
	BlobContentType string
	OwnerEmail      string
	RecipientEmail  string
}
