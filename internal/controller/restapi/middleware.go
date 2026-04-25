package restapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type ctxKey int

const (
	ctxUserID    ctxKey = iota
	ctxSessionID        // device_session_id из JWT claim "sid"
)

// ParseBearerJWT — проверка access JWT.
type ParseBearerJWT interface {
	ParseAccessToken(token string) (userID, deviceSessionID uuid.UUID, err error)
}

// SessionBlocklist проверяет, была ли сессия явно отозвана до истечения токена.
type SessionBlocklist interface {
	IsBlocked(ctx context.Context, id uuid.UUID) (bool, error)
}

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(ctxUserID)
	if v == nil {
		return uuid.UUID{}, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

func SessionIDFromContext(ctx context.Context) uuid.UUID {
	v := ctx.Value(ctxSessionID)
	if v == nil {
		return uuid.Nil
	}
	id, _ := v.(uuid.UUID)
	return id
}

// AuthMiddleware требует заголовок Authorization: Bearer <JWT>.
// Если blocklist != nil, дополнительно проверяет, не была ли сессия явно отозвана.
// При недоступности Redis (ошибка IsBlocked) запрос пропускается — fail open.
func AuthMiddleware(tokens ParseBearerJWT, blocklist SessionBlocklist) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := strings.TrimSpace(r.Header.Get("Authorization"))
			scheme, token, ok := strings.Cut(authz, " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			userID, sessionID, err := tokens.ParseAccessToken(strings.TrimSpace(token))
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if sessionID != uuid.Nil {
				if blocked, err := blocklist.IsBlocked(r.Context(), sessionID); err == nil && blocked {
					WriteError(w, http.StatusUnauthorized, "unauthorized")
					return
				}
			}
			ctx := context.WithValue(r.Context(), ctxUserID, userID)
			ctx = context.WithValue(ctx, ctxSessionID, sessionID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
