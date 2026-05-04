package dto

import "time"

// ─── Blobs ────────────────────────────────────────────────────────────────────

type StoragePresignPutRequest struct {
	FileName         string  `json:"file_name"          validate:"required,max=512"`
	ContentType      string  `json:"content_type"       validate:"required,min=1,max=128"`
	EncryptedFileKey string  `json:"encrypted_file_key" validate:"required"`
	FileIV           string  `json:"file_iv"            validate:"required"`
	FolderID         *string `json:"folder_id"`
}

type StoragePresignPutResponse struct {
	BlobID      string `json:"blob_id"`
	UploadURL   string `json:"upload_url"`
	ExpiresIn   int64  `json:"expires_in"`
	HTTPMethod  string `json:"http_method"`
	ContentType string `json:"content_type"`
}

type StoragePresignGetResponse struct {
	BlobID           string `json:"blob_id"`
	DownloadURL      string `json:"download_url"`
	ExpiresIn        int64  `json:"expires_in"`
	HTTPMethod       string `json:"http_method"`
	ContentType      string `json:"content_type"`
	EncryptedFileKey string `json:"encrypted_file_key"`
	FileIV           string `json:"file_iv"`
}

type StorageBlobItem struct {
	BlobID           string    `json:"blob_id"`
	FolderID         *string   `json:"folder_id"` // null = root level
	FileName         string    `json:"file_name"`
	ContentType      string    `json:"content_type"`
	CreatedAt        time.Time `json:"created_at"`
	EncryptedFileKey string    `json:"encrypted_file_key"`
	FileIV           string    `json:"file_iv"`
}

type StorageListBlobsResponse struct {
	Items []StorageBlobItem `json:"items"`
}

type MoveBlobRequest struct {
	FolderID *string `json:"folder_id"` // null/absent = move to root
}

// ─── Folders ──────────────────────────────────────────────────────────────────

type FolderItem struct {
	FolderID  string    `json:"folder_id"`
	ParentID  *string   `json:"parent_id"` // null = root level
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
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
	ParentID *string `json:"parent_id"` // null/absent = move to root
}
