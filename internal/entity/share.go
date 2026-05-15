package entity

import (
	"time"

	"github.com/google/uuid"
)

// The file key is re-wrapped using ECIES so only the recipient can recover it.
type FileShare struct {
	ID             uuid.UUID
	BlobID         uuid.UUID
	OwnerID        uuid.UUID
	RecipientID    uuid.UUID
	EphemeralPub   []byte // sender's ephemeral P-256 public key (SPKI)
	WrappedFileKey []byte // AES-KW(HKDF(ECDH(ephemeralPriv, recipientPub)), fileKey)
	ExpiresAt      *time.Time
	RevokedAt      *time.Time
	CreatedAt      time.Time
}

// FileShareView is a denormalized read model that adds display-only fields
// (populated via SQL JOINs) to the core FileShare entity.
type FileShareView struct {
	FileShare
	BlobFileName    string
	BlobContentType string
	OwnerEmail      string
	RecipientEmail  string
}
