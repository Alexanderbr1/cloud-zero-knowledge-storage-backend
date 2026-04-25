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

// AuthService — бизнес-логика аутентификации.
type AuthService interface {
	Register(ctx context.Context, email, srpSalt, srpVerifier, bcryptSalt string, cryptoSalt []byte, device authuc.DeviceInfo) (authuc.TokenPair, error)
	LoginInit(ctx context.Context, email, aHex string) (authuc.LoginInitResult, error)
	LoginFinalize(ctx context.Context, sessionID, m1Hex string, device authuc.DeviceInfo) (authuc.LoginFinalizeResult, error)
	Refresh(ctx context.Context, refreshToken string) (authuc.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	ListDeviceSessions(ctx context.Context, userID uuid.UUID) ([]entity.DeviceSession, error)
	RevokeDeviceSession(ctx context.Context, userID, sessionID uuid.UUID) error
	RevokeOtherDeviceSessions(ctx context.Context, userID, currentSessionID uuid.UUID) error
}

// StorageService — бизнес-логика хранилища (реализует usecase/storage.Service).
type StorageService interface {
	PresignPut(ctx context.Context, userID uuid.UUID, fileName, contentType string, encryptedFileKey, fileIV []byte) (*storageuc.PresignPutResult, error)
	PresignGet(ctx context.Context, userID, blobID uuid.UUID) (*storageuc.PresignGetResult, error)
	DeleteBlob(ctx context.Context, userID, blobID uuid.UUID) error
	ListBlobs(ctx context.Context, userID uuid.UUID) ([]entity.Blob, error)
}

// Deps — зависимости v1-роутера; все поля — интерфейсы для тестируемости.
type Deps struct {
	Auth          AuthService
	Tokens        restapi.ParseBearerJWT
	Sessions      restapi.SessionBlocklist
	Storage       StorageService
	RefreshCookie config.RefreshCookieConfig
}

func NewRouter(d Deps) chi.Router {
	r := chi.NewRouter()

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", register(d))
		r.Post("/login/init", loginInit(d))
		r.Post("/login/finalize", loginFinalize(d))
		r.Post("/refresh", refresh(d))
		r.Post("/logout", logout(d))
	})

	r.Group(func(r chi.Router) {
		r.Use(restapi.AuthMiddleware(d.Tokens, d.Sessions))

		r.Route("/auth/sessions", func(r chi.Router) {
			r.Get("/", listSessions(d))
			r.Delete("/", revokeOtherSessions(d))
			r.Delete("/{id}", revokeSession(d))
		})

		if d.Storage != nil {
			r.Route("/storage", func(r chi.Router) {
				registerStorageRoutes(r, d)
			})
		}
	})

	return r
}
