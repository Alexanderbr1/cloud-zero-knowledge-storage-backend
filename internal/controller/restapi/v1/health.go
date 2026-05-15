package v1

import (
	"net/http"

	"cloud-backend/internal/controller/restapi"
)

func health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		restapi.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
