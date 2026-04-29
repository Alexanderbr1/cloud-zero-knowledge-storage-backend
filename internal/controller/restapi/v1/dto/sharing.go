package dto

import "time"

// GetPublicKeyResponse — public key of a user, used by sender before creating a share.
type GetPublicKeyResponse struct {
	PublicKey string `json:"public_key"` // base64-encoded SPKI P-256 public key
}

// CreateShareRequest — owner creates a share for a recipient.
// All crypto is done client-side; the server only stores the opaque blobs.
type CreateShareRequest struct {
	RecipientEmail string  `json:"recipient_email" validate:"required,email,max=320"`
	EphemeralPub   string  `json:"ephemeral_pub"   validate:"required"`  // base64 SPKI of ephemeral EC key
	WrappedFileKey string  `json:"wrapped_file_key" validate:"required"` // base64 AES-KW(KEK, fileKey)
	ExpiresAt      *string `json:"expires_at,omitempty"`                 // RFC3339 optional expiry
}

// ShareResponse — a single share record returned to the caller.
type ShareResponse struct {
	ShareID         string    `json:"share_id"`
	BlobID          string    `json:"blob_id"`
	OwnerEmail      string    `json:"owner_email"`
	RecipientEmail  string    `json:"recipient_email,omitempty"`
	BlobFileName    string    `json:"file_name"`
	BlobContentType string    `json:"content_type"`
	EphemeralPub    string    `json:"ephemeral_pub"`    // base64
	WrappedFileKey  string    `json:"wrapped_file_key"` // base64
	ExpiresAt       *string   `json:"expires_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	// DownloadURL is populated only for GetSharedFile.
	DownloadURL string `json:"download_url,omitempty"`
	FileIV      string `json:"file_iv,omitempty"` // base64, only for GetSharedFile
}

type ListSharesResponse struct {
	Items []ShareResponse `json:"items"`
}
