package entity

import (
	"time"

	"github.com/google/uuid"
)

// Blob — метаданные файла в объектном хранилище.
type Blob struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	FileName  string
	ObjectKey string
	CreatedAt time.Time
}
