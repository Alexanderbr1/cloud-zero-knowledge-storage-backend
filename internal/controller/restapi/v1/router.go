package v1

import (
	"github.com/go-chi/chi/v5"

	"cloud-backend/config"
	"cloud-backend/internal/controller/restapi"
	authuc "cloud-backend/internal/usecase/auth"
	storageuc "cloud-backend/internal/usecase/storage"
	jwtpkg "cloud-backend/pkg/jwt"
)

type Deps struct {
	Auth          *authuc.Service
	Tokens        *jwtpkg.Service
	Storage       *storageuc.Service
	RefreshCookie config.RefreshCookieConfig
}

func NewRouter(d Deps) chi.Router {
	r := chi.NewRouter()

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", register(d))
		r.Post("/login", login(d))
		r.Post("/refresh", refresh(d))
		r.Post("/logout", logout(d))
	})

	if d.Storage != nil {
		r.Group(func(r chi.Router) {
			r.Use(restapi.AuthMiddleware(d.Tokens))
			r.Route("/storage", func(r chi.Router) {
				registerStorageRoutes(r, d)
			})
		})
	}

	return r
}
