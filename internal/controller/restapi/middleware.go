package restapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

type ParseBearerJWT interface {
	ParseAccessToken(token string) (userID, deviceSessionID uuid.UUID, err error)
}

type SessionBlocklist interface {
	IsBlocked(ctx context.Context, id uuid.UUID) (bool, error)
}

type ctxKey int

const (
	ctxUserID    ctxKey = iota
	ctxSessionID        // device_session_id из JWT claim "sid"
)

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxUserID).(uuid.UUID)
	return id, ok
}

func SessionIDFromContext(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(ctxSessionID).(uuid.UUID)
	return id
}

func MustUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := UserIDFromContext(r.Context())
	if !ok || id == uuid.Nil {
		WriteError(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, false
	}
	return id, true
}

func RateLimitMiddleware(rl RateLimiter, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, err := rl.Allow(r.Context(), keyFn(r))
			if err != nil {
				WriteError(w, http.StatusServiceUnavailable, "service unavailable")
				return
			}
			if !allowed {
				WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

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
					log.Error().Err(err).Msg("blocklist check failed; denying access")
					WriteError(w, http.StatusServiceUnavailable, "service unavailable")
					return
				}
				if blocked {
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
