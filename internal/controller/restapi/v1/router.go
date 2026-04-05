package v1

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cloud-backend/config"
	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/entity"
	authuc "cloud-backend/internal/usecase/auth"
	storageuc "cloud-backend/internal/usecase/storage"
)

// AuthService — бизнес-логика аутентификации (реализует usecase/auth.Service).
type AuthService interface {
	Register(ctx context.Context, email, password string, cryptoSalt []byte) (authuc.TokenPair, error)
	Login(ctx context.Context, email, password string) (authuc.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (authuc.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	GetCryptoParams(ctx context.Context, email string) (cryptoSalt []byte, ok bool, err error)
}

// StorageService — бизнес-логика хранилища (реализует usecase/storage.Service).
type StorageService interface {
	PresignPut(ctx context.Context, userID uuid.UUID, fileName string, encryptedFileKey, fileIV []byte) (*storageuc.PresignPutResult, error)
	PresignGet(ctx context.Context, userID, blobID uuid.UUID) (*storageuc.PresignGetResult, error)
	DeleteBlob(ctx context.Context, userID, blobID uuid.UUID) error
	ListBlobs(ctx context.Context, userID uuid.UUID) ([]entity.Blob, error)
}

// Deps — зависимости v1-роутера; все поля — интерфейсы для тестируемости.
type Deps struct {
	Auth          AuthService
	Tokens        restapi.ParseBearerJWT
	Storage       StorageService
	RefreshCookie config.RefreshCookieConfig
}

func NewRouter(d Deps) chi.Router {
	r := chi.NewRouter()

	r.Route("/auth", func(r chi.Router) {
		r.Get("/crypto-params", getCryptoParams(d)) // публичный, до логина
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
