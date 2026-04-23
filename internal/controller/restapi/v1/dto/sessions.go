package dto

import (
	"time"

	"github.com/google/uuid"
)

type DeviceSessionDTO struct {
	ID           uuid.UUID `json:"id"`
	DeviceName   string    `json:"device_name"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	IsCurrent    bool      `json:"is_current"`
}

type ListSessionsResponse struct {
	Sessions []DeviceSessionDTO `json:"sessions"`
}
