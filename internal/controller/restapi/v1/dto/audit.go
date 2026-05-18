package dto

import (
	"time"

	"github.com/google/uuid"
)

type AuditEventItem struct {
	ID           uuid.UUID  `json:"id"`
	EventType    string     `json:"event_type"`
	IPAddress    string     `json:"ip_address"`
	DeviceName   string     `json:"device_name"`
	ResourceID   *uuid.UUID `json:"resource_id,omitempty"`
	ResourceName string     `json:"resource_name,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type ListAuditResponse struct {
	Events []AuditEventItem `json:"events"`
}
