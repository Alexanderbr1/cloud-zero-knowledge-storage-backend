package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	v1 "cloud-backend/internal/controller/restapi/v1"
)

// newHTTPHandler собирает chi-router для REST (delivery), как router в go-clean-template.
func newHTTPHandler(apiV1 v1.Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	// Не задаём глобальный Timeout: загрузка blob'ов может быть долгой; лимиты — http.Server + /storage Timeout.
	r.Use(middleware.Logger)
	r.Route("/v1", func(r chi.Router) {
		r.Mount("/", v1.NewRouter(apiV1))
	})
	return r
}
