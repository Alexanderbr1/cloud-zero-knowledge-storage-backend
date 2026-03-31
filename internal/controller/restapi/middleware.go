package restapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type ctxKey int

const ctxUserID ctxKey = iota

// ParseBearerJWT — проверка access JWT (реализует pkg/jwt.Service).
type ParseBearerJWT interface {
	ParseAccessToken(token string) (uuid.UUID, error)
}

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(ctxUserID)
	if v == nil {
		return uuid.UUID{}, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

// AuthMiddleware требует заголовок Authorization: Bearer <JWT>.
func AuthMiddleware(tokens ParseBearerJWT) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := strings.TrimSpace(r.Header.Get("Authorization"))
			scheme, token, ok := strings.Cut(authz, " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			raw := strings.TrimSpace(token)
			userID, err := tokens.ParseAccessToken(raw)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx := context.WithValue(r.Context(), ctxUserID, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
