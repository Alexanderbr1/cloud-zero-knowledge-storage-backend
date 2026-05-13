package entity

import (
	"time"

	"github.com/google/uuid"
)

// Blob — метаданные файла в объектном хранилище.
type Blob struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	FolderID         *uuid.UUID
	FileName         string
	ContentType      string
	ObjectKey        string
	FileSize         int64
	CreatedAt        time.Time
	EncryptedFileKey []byte // AES-KW обёрнутый файловый ключ
	FileIV           []byte // 12-байтовый IV для AES-GCM

	// Trash fields — non-nil only for items in the recycle bin.
	DeletedAt        *time.Time
	OriginalFolderID *uuid.UUID
}
