package dto

import "time"

type PresignGetResponse struct {
	BlobID           string `json:"blob_id"`
	DownloadURL      string `json:"download_url"`
	ExpiresIn        int64  `json:"expires_in"`
	HTTPMethod       string `json:"http_method"`
	ContentType      string `json:"content_type"`
	EncryptedFileKey string `json:"encrypted_file_key"` // base64
	FileSize         int64  `json:"file_size"`
	FileSizePlain    int64  `json:"file_size_plain"`
	ChunkSize        int32  `json:"chunk_size"`
}

type BlobItem struct {
	BlobID           string    `json:"blob_id"`
	FolderID         *string   `json:"folder_id"` // null = root
	FileName         string    `json:"file_name"`
	ContentType      string    `json:"content_type"`
	FileSize         int64     `json:"file_size"`
	FileSizePlain    int64     `json:"file_size_plain"`
	ChunkSize        int32     `json:"chunk_size"`
	CreatedAt        time.Time `json:"created_at"`
	EncryptedFileKey string    `json:"encrypted_file_key"` // base64
}

type ListBlobsResponse struct {
	Items []BlobItem `json:"items"`
}

// ─── Multipart upload ─────────────────────────────────────────────────────────

type InitiateMultipartRequest struct {
	FileName         string  `json:"file_name"          validate:"required,max=512"`
	ContentType      string  `json:"content_type"       validate:"required,min=1,max=128"`
	EncryptedFileKey string  `json:"encrypted_file_key" validate:"required"`
	FileSize         int64   `json:"file_size"          validate:"required,min=1"`
	FileSizePlain    int64   `json:"file_size_plain"    validate:"required,min=1"`
	ChunkSize        int32   `json:"chunk_size"         validate:"required,min=1"`
	PartCount        int     `json:"part_count"         validate:"required,min=1,max=10000"`
	FolderID         *string `json:"folder_id"`
}

type PartURLItem struct {
	PartNumber int    `json:"part_number"`
	URL        string `json:"url"`
}

type InitiateMultipartResponse struct {
	BlobID   string        `json:"blob_id"`
	UploadID string        `json:"upload_id"`
	PartURLs []PartURLItem `json:"part_urls"`
}

type UploadedPart struct {
	PartNumber int    `json:"part_number" validate:"required,min=1,max=10000"`
	ETag       string `json:"etag"        validate:"required"`
}

type CompleteMultipartRequest struct {
	Parts []UploadedPart `json:"parts" validate:"required,min=1,dive"`
}

// ─── Move / Rename ────────────────────────────────────────────────────────────

type MoveBlobRequest struct {
	FolderID *string `json:"folder_id"` // null = move to root
}

type RenameBlobRequest struct {
	Name string `json:"name" validate:"required,min=1,max=512"`
}

type FolderItem struct {
	FolderID  string    `json:"folder_id"`
	ParentID  *string   `json:"parent_id"` // null = root
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type SearchBlobItem struct {
	BlobID           string    `json:"blob_id"`
	FolderID         *string   `json:"folder_id"`
	FolderName       *string   `json:"folder_name"`
	FileName         string    `json:"file_name"`
	ContentType      string    `json:"content_type"`
	FileSize         int64     `json:"file_size"`
	FileSizePlain    int64     `json:"file_size_plain"`
	ChunkSize        int32     `json:"chunk_size"`
	CreatedAt        time.Time `json:"created_at"`
	EncryptedFileKey string    `json:"encrypted_file_key"` // base64
}

type SearchResponse struct {
	Blobs   []SearchBlobItem `json:"blobs"`
	Folders []FolderItem     `json:"folders"`
}

type ListFoldersResponse struct {
	Items []FolderItem `json:"items"`
}

type CreateFolderRequest struct {
	Name     string  `json:"name"      validate:"required,min=1,max=255"`
	ParentID *string `json:"parent_id"`
}

type RenameFolderRequest struct {
	Name string `json:"name" validate:"required,min=1,max=255"`
}

type MoveFolderRequest struct {
	ParentID *string `json:"parent_id"` // null = move to root
}

type TrashBlobItem struct {
	BlobID           string    `json:"blob_id"`
	FolderID         *string   `json:"folder_id"`
	FileName         string    `json:"file_name"`
	ContentType      string    `json:"content_type"`
	FileSize         int64     `json:"file_size"`
	FileSizePlain    int64     `json:"file_size_plain"`
	ChunkSize        int32     `json:"chunk_size"`
	CreatedAt        time.Time `json:"created_at"`
	EncryptedFileKey string    `json:"encrypted_file_key"` // base64
}

type TrashFolderItem struct {
	FolderID  string    `json:"folder_id"`
	ParentID  *string   `json:"parent_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type TrashListResponse struct {
	Blobs   []TrashBlobItem   `json:"blobs"`
	Folders []TrashFolderItem `json:"folders"`
}

type FavoritesResponse struct {
	Blobs   []SearchBlobItem `json:"blobs"`
	Folders []FolderItem     `json:"folders"`
}
