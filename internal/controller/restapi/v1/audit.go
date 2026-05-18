package v1

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/controller/restapi/v1/dto"
	"cloud-backend/internal/entity"
	"cloud-backend/pkg/useragent"
)

type AuditService interface {
	LogAsync(ctx context.Context, e entity.AuditEvent)
	ListEvents(ctx context.Context, userID uuid.UUID, limit int, before time.Time) ([]entity.AuditEvent, error)
}

func listAuditEvents(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := restapi.MustUserID(w, r)
		if !ok {
			return
		}

		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
				limit = n
			}
		}

		var before time.Time
		if b := r.URL.Query().Get("before"); b != "" {
			before, _ = time.Parse(time.RFC3339Nano, b)
		}

		events, err := d.Audit.ListEvents(r.Context(), userID, limit, before)
		if err != nil {
			restapi.WriteInternalError(w, d.Logger, err)
			return
		}

		out := make([]dto.AuditEventItem, 0, len(events))
		for _, e := range events {
			out = append(out, dto.AuditEventItem{
				ID:           e.ID,
				EventType:    e.EventType,
				IPAddress:    e.IPAddress,
				DeviceName:   useragent.Parse(e.UserAgent),
				ResourceID:   e.ResourceID,
				ResourceName: e.ResourceName,
				CreatedAt:    e.CreatedAt,
			})
		}
		restapi.WriteJSON(w, http.StatusOK, dto.ListAuditResponse{Events: out})
	}
}
