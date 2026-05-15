package restapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/rs/zerolog"
)

const maxJSONBodyBytes = 4 << 20 // 4 MiB

type errorResponse struct {
	Error string `json:"error"`
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, errorResponse{Error: msg})
}

func WriteInternalError(w http.ResponseWriter, log zerolog.Logger, err error) {
	log.Error().Err(err).Msg("internal server error")
	WriteJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
}

func DecodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	return json.NewDecoder(io.LimitReader(r.Body, maxJSONBodyBytes)).Decode(dst)
}
