package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	v1 "cloud-backend/internal/controller/restapi/v1"
)

// newHTTPHandler собирает chi-роутер с глобальными middleware.
// Глобальный Timeout не задаётся: загрузки blob'ов длинные; лимиты — http.Server + /storage Timeout.
func newHTTPHandler(deps v1.Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	r.Mount("/v1", v1.NewRouter(deps))
	return r
}
