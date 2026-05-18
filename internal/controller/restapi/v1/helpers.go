package v1

import (
	"encoding/base64"
	"net/http"

	"github.com/google/uuid"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/entity"
)

func mustDecodeB64(w http.ResponseWriter, b64, field string, minLen, maxLen int) ([]byte, bool) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || (minLen > 0 && len(raw) < minLen) || (maxLen > 0 && len(raw) > maxLen) {
		restapi.WriteError(w, http.StatusBadRequest, "invalid "+field)
		return nil, false
	}
	return raw, true
}

func auditEvent(userID uuid.UUID, eventType, ip, ua string, resourceID *uuid.UUID, resourceName string) entity.AuditEvent {
	return entity.AuditEvent{
		UserID:       userID,
		EventType:    eventType,
		IPAddress:    ip,
		UserAgent:    ua,
		ResourceID:   resourceID,
		ResourceName: resourceName,
	}
}
