package entity

import (
	"time"

	"github.com/google/uuid"
)

type Folder struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ParentID  *uuid.UUID
	Name      string
	TotalSize int64
	CreatedAt time.Time
}
