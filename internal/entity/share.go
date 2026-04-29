package entity

import (
	"time"

	"github.com/google/uuid"
)

// FileShare — record of a file shared from one user to another.
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
	// Populated in list queries for display purposes.
	BlobFileName    string
	BlobContentType string
	OwnerEmail      string
	RecipientEmail  string
}
