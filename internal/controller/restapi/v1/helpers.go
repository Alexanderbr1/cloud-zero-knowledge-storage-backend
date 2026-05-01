package v1

import (
	"encoding/base64"
	"net/http"

	"cloud-backend/internal/controller/restapi"
)

// mustDecodeB64 decodes a standard base64 field and validates its length.
// minLen ≤ 0 means no lower bound; maxLen ≤ 0 means no upper bound.
// Writes 400 and returns false on any failure — caller must return immediately.
func mustDecodeB64(w http.ResponseWriter, b64, field string, minLen, maxLen int) ([]byte, bool) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || (minLen > 0 && len(raw) < minLen) || (maxLen > 0 && len(raw) > maxLen) {
		restapi.WriteError(w, http.StatusBadRequest, "invalid "+field)
		return nil, false
	}
	return raw, true
}
