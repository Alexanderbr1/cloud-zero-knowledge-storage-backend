// Package restapi — REST delivery layer (аналог internal/controller/restapi в go-clean-template).
package restapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// maxJSONBodyBytes — защита от чрезмерно больших JSON-тел.
const maxJSONBodyBytes = 4 << 20 // 4 MiB

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]any{"error": msg})
}

// DecodeJSON читает тело запроса как JSON с ограничением размера (тело закрывает net/http после хендлера).
func DecodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, maxJSONBodyBytes))
	return dec.Decode(dst)
}
