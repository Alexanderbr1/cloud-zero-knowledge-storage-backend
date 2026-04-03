package dto

import "time"

type StoragePresignPutRequest struct {
	FileName string `json:"file_name" validate:"required,max=512"`
}

type StoragePresignPutResponse struct {
	BlobID       string `json:"blob_id"`
	UploadURL    string `json:"upload_url"`
	ExpiresIn    int64  `json:"expires_in"`
	HTTPMethod   string `json:"http_method"`
	Instructions string `json:"instructions"`
}

type StoragePresignGetResponse struct {
	BlobID      string `json:"blob_id"`
	DownloadURL string `json:"download_url"`
	ExpiresIn   int64  `json:"expires_in"`
	HTTPMethod  string `json:"http_method"`
}

type StorageBlobItem struct {
	BlobID    string    `json:"blob_id"`
	FileName  string    `json:"file_name"`
	CreatedAt time.Time `json:"created_at"`
}

type StorageListBlobsResponse struct {
	Items []StorageBlobItem `json:"items"`
}
