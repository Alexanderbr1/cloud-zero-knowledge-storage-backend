// Package restapi — REST delivery layer.
package restapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// maxJSONBodyBytes — защита от чрезмерно больших JSON-тел.
const maxJSONBodyBytes = 4 << 20 // 4 MiB

type errorResponse struct {
	Error string `json:"error"`
}

type fieldError struct {
	Field string `json:"field"`
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

type validationErrorResponse struct {
	Error  string       `json:"error"`
	Fields []fieldError `json:"fields"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, errorResponse{Error: msg})
}

// DecodeJSON читает тело запроса как JSON с ограничением размера.
func DecodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	return json.NewDecoder(io.LimitReader(r.Body, maxJSONBodyBytes)).Decode(dst)
}
