package v1

import (
	"github.com/go-chi/chi/v5"

	"cloud-backend/internal/controller/restapi"
	authuc "cloud-backend/internal/usecase/auth"
	storageuc "cloud-backend/internal/usecase/storage"
	jwtpkg "cloud-backend/pkg/jwt"
)

// Deps — зависимости HTTP-слоя (инъекция из app).
type Deps struct {
	Auth    *authuc.Service
	Tokens  *jwtpkg.Service
	Storage *storageuc.Service
}

func NewRouter(d Deps) chi.Router {
	r := chi.NewRouter()

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", registerAuth(d))
		r.Post("/login", loginAuth(d))
		r.Post("/refresh", refreshAuth(d))
		r.Post("/logout", logoutAuth(d))
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
