package dto

import "time"

type PresignPutRequest struct {
	FileName         string  `json:"file_name"          validate:"required,max=512"`
	ContentType      string  `json:"content_type"       validate:"required,min=1,max=128"`
	EncryptedFileKey string  `json:"encrypted_file_key" validate:"required"` // base64
	FileIV           string  `json:"file_iv"            validate:"required"` // base64
	FolderID         *string `json:"folder_id"`
	FileSize         int64   `json:"file_size"           validate:"required,min=1"`
}

type PresignPutResponse struct {
	BlobID      string `json:"blob_id"`
	UploadURL   string `json:"upload_url"`
	ExpiresIn   int64  `json:"expires_in"`
	HTTPMethod  string `json:"http_method"`
	ContentType string `json:"content_type"`
}

type PresignGetResponse struct {
	BlobID           string `json:"blob_id"`
	DownloadURL      string `json:"download_url"`
	ExpiresIn        int64  `json:"expires_in"`
	HTTPMethod       string `json:"http_method"`
	ContentType      string `json:"content_type"`
	EncryptedFileKey string `json:"encrypted_file_key"` // base64
	FileIV           string `json:"file_iv"`            // base64
}

type BlobItem struct {
	BlobID           string    `json:"blob_id"`
	FolderID         *string   `json:"folder_id"` // null = root
	FileName         string    `json:"file_name"`
	ContentType      string    `json:"content_type"`
	FileSize         int64     `json:"file_size"`
	CreatedAt        time.Time `json:"created_at"`
	EncryptedFileKey string    `json:"encrypted_file_key"` // base64
	FileIV           string    `json:"file_iv"`            // base64
}

type ListBlobsResponse struct {
	Items []BlobItem `json:"items"`
}

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
	CreatedAt        time.Time `json:"created_at"`
	EncryptedFileKey string    `json:"encrypted_file_key"` // base64
	FileIV           string    `json:"file_iv"`            // base64
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
	CreatedAt        time.Time `json:"created_at"`
	EncryptedFileKey string    `json:"encrypted_file_key"` // base64
	FileIV           string    `json:"file_iv"`            // base64
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
