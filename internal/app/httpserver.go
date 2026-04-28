package app

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	v1 "cloud-backend/internal/controller/restapi/v1"
)

// newHTTPHandler собирает chi-роутер с глобальными middleware.
// Глобальный Timeout не задаётся: загрузки blob'ов длинные; лимиты — http.Server + /storage Timeout.
func newHTTPHandler(deps v1.Deps, log zerolog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(log))
	r.Mount("/v1", v1.NewRouter(deps))
	return r
}

func requestLogger(log zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			log.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.Status()).
				Dur("latency", time.Since(start)).
				Str("request_id", middleware.GetReqID(r.Context())).
				Msg("request")
		})
	}
}
