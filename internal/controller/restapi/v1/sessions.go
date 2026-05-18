package v1

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	"cloud-backend/internal/entity"
	authuc "cloud-backend/internal/usecase/auth"
)

type SessionsService interface {
	ListDeviceSessions(ctx context.Context, userID uuid.UUID) ([]entity.DeviceSession, error)
	RevokeDeviceSession(ctx context.Context, userID, sessionID uuid.UUID) error
	RevokeOtherDeviceSessions(ctx context.Context, userID, currentSessionID uuid.UUID) error
}

func listSessions(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		currentSessionID := restapi.SessionIDFromContext(r.Context())

		sessions, err := d.DeviceSessions.ListDeviceSessions(r.Context(), userID)
		if err != nil {
			restapi.WriteInternalError(w, d.Logger, err)
			return
		}

		out := make([]dto.DeviceSession, 0, len(sessions))
		for _, s := range sessions {
			out = append(out, sessionToDTO(s, currentSessionID))
		}
		restapi.WriteJSON(w, http.StatusOK, dto.ListSessionsResponse{Sessions: out})
	}
}

func revokeSession(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}

		sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			restapi.WriteError(w, http.StatusBadRequest, "invalid session id")
			return
		}

		if err := d.DeviceSessions.RevokeDeviceSession(r.Context(), userID, sessionID); err != nil {
			if errors.Is(err, authuc.ErrSessionNotFound) {
				restapi.WriteError(w, http.StatusNotFound, "session not found")
				return
			}
			restapi.WriteInternalError(w, d.Logger, err)
			return
		}
		d.Audit.LogAsync(r.Context(), auditEvent(userID, entity.AuditSessionRevoked, realIP(r), r.Header.Get("User-Agent"), &sessionID, ""))
		w.WriteHeader(http.StatusNoContent)
	}
}

func revokeOtherSessions(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}
		currentSessionID := restapi.SessionIDFromContext(r.Context())

		if err := d.DeviceSessions.RevokeOtherDeviceSessions(r.Context(), userID, currentSessionID); err != nil {
			restapi.WriteInternalError(w, d.Logger, err)
			return
		}
		d.Audit.LogAsync(r.Context(), auditEvent(userID, entity.AuditSessionsRevoked, realIP(r), r.Header.Get("User-Agent"), nil, ""))
		w.WriteHeader(http.StatusNoContent)
	}
}

func sessionToDTO(s entity.DeviceSession, currentID uuid.UUID) dto.DeviceSession {
	return dto.DeviceSession{
		ID:           s.ID,
		DeviceName:   s.DeviceName,
		IPAddress:    s.IPAddress,
		UserAgent:    s.UserAgent,
		CreatedAt:    s.CreatedAt,
		LastActiveAt: s.LastActiveAt,
		IsCurrent:    s.ID == currentID,
	}
}
