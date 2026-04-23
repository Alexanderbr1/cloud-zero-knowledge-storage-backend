package entity

import (
	"time"

	"github.com/google/uuid"
)

// DeviceSession — активная пользовательская сессия на конкретном устройстве.
// Переживает ротацию refresh-токенов: last_active_at обновляется при каждом refresh,
// но запись остаётся той же.
type DeviceSession struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	DeviceName   string // "Chrome on macOS"
	IPAddress    string // IP при первом логине
	UserAgent    string // raw User-Agent
	CreatedAt    time.Time
	LastActiveAt time.Time
	RevokedAt    *time.Time // nil — сессия активна
}
