package entity

import (
	"time"

	"github.com/google/uuid"
)

// Blob — метаданные файла в объектном хранилище.
type Blob struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	FileName         string
	ContentType      string
	ObjectKey        string
	CreatedAt        time.Time
	EncryptedFileKey []byte // AES-KW обёрнутый файловый ключ; nil у незашифрованных файлов
	FileIV           []byte // 12-байтовый IV для AES-GCM; nil у незашифрованных файлов
}
