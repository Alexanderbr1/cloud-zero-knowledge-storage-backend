package dto

import "time"

// StoragePresignPutRequest — POST /v1/storage/presign
type StoragePresignPutRequest struct {
	ContentType string `json:"content_type" validate:"omitempty,max=256"`
	FileName    string `json:"file_name" validate:"required,max=512"`
}

// StoragePresignPutResponse — ответ presign (PUT в объектное хранилище)
type StoragePresignPutResponse struct {
	BlobID       string `json:"blob_id"`
	ObjectKey    string `json:"object_key"`
	UploadURL    string `json:"upload_url"`
	ExpiresIn    int64  `json:"expires_in"`
	HTTPMethod   string `json:"http_method"`
	ContentType  string `json:"content_type"`
	Instructions string `json:"instructions"`
}

// StoragePresignGetResponse — POST /v1/storage/blobs/{id}/presign-get
type StoragePresignGetResponse struct {
	BlobID       string `json:"blob_id"`
	ObjectKey    string `json:"object_key"`
	DownloadURL  string `json:"download_url"`
	ExpiresIn    int64  `json:"expires_in"`
	HTTPMethod   string `json:"http_method"`
	ContentType  string `json:"content_type"`
	Instructions string `json:"instructions"`
}

type StorageBlobItem struct {
	BlobID      string    `json:"blob_id"`
	FileName    string    `json:"file_name"`
	ObjectKey   string    `json:"object_key"`
	ContentType string    `json:"content_type,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type StorageListBlobsResponse struct {
	Items []StorageBlobItem `json:"items"`
}
