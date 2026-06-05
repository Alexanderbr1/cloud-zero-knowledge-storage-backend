package dto

import "time"

type GetPublicKeyResponse struct {
	PublicKey string `json:"public_key"` // base64 SPKI P-256
}

type CreateShareRequest struct {
	RecipientEmail string  `json:"recipient_email"  validate:"required,email,max=320"`
	EphemeralPub   string  `json:"ephemeral_pub"    validate:"required"` // base64 SPKI ephemeral EC key
	WrappedFileKey string  `json:"wrapped_file_key" validate:"required"` // base64 AES-KW(KEK, fileKey)
	ExpiresAt      *string `json:"expires_at,omitempty"`                 // RFC3339
}

type ShareResponse struct {
	ShareID         string    `json:"share_id"`
	BlobID          string    `json:"blob_id"`
	OwnerID         string    `json:"owner_id"`
	OwnerEmail      string    `json:"owner_email"`
	RecipientEmail  string    `json:"recipient_email"`
	BlobFileName    string    `json:"file_name"`
	BlobContentType string    `json:"content_type"`
	EphemeralPub    string    `json:"ephemeral_pub"`    // base64
	WrappedFileKey  string    `json:"wrapped_file_key"` // base64
	ExpiresAt       *string   `json:"expires_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	DownloadURL     string    `json:"download_url,omitempty"`    // GetSharedFile only
	FileSize        int64     `json:"file_size,omitempty"`       // GetSharedFile only
	FileSizePlain   int64     `json:"file_size_plain,omitempty"` // GetSharedFile only
	ChunkSize       int32     `json:"chunk_size,omitempty"`      // GetSharedFile only
}

type ListSharesResponse struct {
	Items []ShareResponse `json:"items"`
}
