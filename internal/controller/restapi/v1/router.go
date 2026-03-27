package v1

import "github.com/go-chi/chi/v5"

import storageuc "cloud-backend/internal/usecase/storage"

// Deps — зависимости HTTP-слоя (инъекция из app).
type Deps struct {
	// Storage — загрузка blob'ов в MinIO; nil если MINIO_ENDPOINT не задан.
	Storage *storageuc.Service
}

func NewRouter(d Deps) chi.Router {
	r := chi.NewRouter()
	if d.Storage != nil {
		r.Route("/storage", func(r chi.Router) {
			registerStorageRoutes(r, d)
		})
	}
	return r
}
