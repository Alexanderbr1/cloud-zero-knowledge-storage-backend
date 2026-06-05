package entity

import (
	"time"

	"github.com/google/uuid"
)

type DeviceSession struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	DeviceName   string
	IPAddress    string
	UserAgent    string
	CreatedAt    time.Time
	LastActiveAt time.Time
	RevokedAt    *time.Time
}
