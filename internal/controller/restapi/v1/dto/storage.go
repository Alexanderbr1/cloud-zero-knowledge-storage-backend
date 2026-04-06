package dto

import "time"

type StoragePresignPutRequest struct {
	FileName         string `json:"file_name"          validate:"required,max=512"`
	ContentType      string `json:"content_type"       validate:"required,min=1,max=128"`
	EncryptedFileKey string `json:"encrypted_file_key" validate:"required"`
	FileIV           string `json:"file_iv"            validate:"required"`
}

type StoragePresignPutResponse struct {
	BlobID       string `json:"blob_id"`
	UploadURL    string `json:"upload_url"`
	ExpiresIn    int64  `json:"expires_in"`
	HTTPMethod   string `json:"http_method"`
	ContentType  string `json:"content_type"`
	Instructions string `json:"instructions"`
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
	FileName         string    `json:"file_name"`
	ContentType      string    `json:"content_type"`
	CreatedAt        time.Time `json:"created_at"`
	EncryptedFileKey string    `json:"encrypted_file_key"`
	FileIV           string    `json:"file_iv"`
}

type StorageListBlobsResponse struct {
	Items []StorageBlobItem `json:"items"`
}
