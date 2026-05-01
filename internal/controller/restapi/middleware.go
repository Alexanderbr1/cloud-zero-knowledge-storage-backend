package restapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// RateLimiter is the interface required by RateLimitMiddleware.
type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

// RateLimitMiddleware: keyFn extracts the bucket key (e.g. user ID, IP); fails open on limiter error.
func RateLimitMiddleware(rl RateLimiter, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, _ := rl.Allow(r.Context(), keyFn(r))
			if !allowed {
				WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type ctxKey int

const (
	ctxUserID    ctxKey = iota
	ctxSessionID        // device_session_id из JWT claim "sid"
)

type ParseBearerJWT interface {
	ParseAccessToken(token string) (userID, deviceSessionID uuid.UUID, err error)
}

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

// MustUserID извлекает ID аутентифицированного пользователя из контекста.
// При отсутствии пишет 401 и возвращает false — вызывающий должен сделать return.
// В норме не срабатывает: AuthMiddleware гарантирует наличие ID на защищённых маршрутах.
func MustUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := UserIDFromContext(r.Context())
	if !ok || id == uuid.Nil {
		WriteError(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, false
	}
	return id, true
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
// При недоступности Redis (ошибка IsBlocked) запрос пропускается — fail open, ошибка логируется.
func AuthMiddleware(tokens ParseBearerJWT, blocklist SessionBlocklist, log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := strings.TrimSpace(r.Header.Get("Authorization"))
			scheme, token, ok := strings.Cut(authz, " ")
			token = strings.TrimSpace(token)
			if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			userID, sessionID, err := tokens.ParseAccessToken(token)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if sessionID != uuid.Nil {
				blocked, err := blocklist.IsBlocked(r.Context(), sessionID)
				if err != nil {
					log.Warn().Err(err).Msg("blocklist check failed; failing open")
				} else if blocked {
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
