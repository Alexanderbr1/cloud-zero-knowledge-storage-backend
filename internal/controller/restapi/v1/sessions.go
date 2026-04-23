package v1

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	authuc "cloud-backend/internal/usecase/auth"
)

func listSessions(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := restapi.UserIDFromContext(r.Context())
		if !ok {
			restapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		currentSessionID := restapi.SessionIDFromContext(r.Context())

		sessions, err := d.Auth.ListDeviceSessions(r.Context(), userID)
		if err != nil {
			restapi.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}

		out := make([]dto.DeviceSessionDTO, 0, len(sessions))
		for _, s := range sessions {
			out = append(out, dto.DeviceSessionDTO{
				ID:           s.ID,
				DeviceName:   s.DeviceName,
				IPAddress:    s.IPAddress,
				UserAgent:    s.UserAgent,
				CreatedAt:    s.CreatedAt,
				LastActiveAt: s.LastActiveAt,
				IsCurrent:    s.ID == currentSessionID,
			})
		}
		restapi.WriteJSON(w, http.StatusOK, dto.ListSessionsResponse{Sessions: out})
	}
}

func revokeSession(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := restapi.UserIDFromContext(r.Context())
		if !ok {
			restapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid session id")
			return
		}

		if err := d.Auth.RevokeDeviceSession(r.Context(), userID, sessionID); err != nil {
			if errors.Is(err, authuc.ErrSessionNotFound) {
				restapi.WriteError(w, http.StatusNotFound, "session not found")
				return
			}
			restapi.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func revokeOtherSessions(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := restapi.UserIDFromContext(r.Context())
		if !ok {
			restapi.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		currentSessionID := restapi.SessionIDFromContext(r.Context())

		if err := d.Auth.RevokeOtherDeviceSessions(r.Context(), userID, currentSessionID); err != nil {
			restapi.WriteError(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
