package v1

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"cloud-backend/config"
	"cloud-backend/internal/controller/restapi"
	"cloud-backend/internal/entity"
	authuc "cloud-backend/internal/usecase/auth"
	sharinguc "cloud-backend/internal/usecase/sharing"
	storageuc "cloud-backend/internal/usecase/storage"
)

// AuthService — бизнес-логика аутентификации.
type AuthService interface {
	Register(ctx context.Context, p authuc.RegisterParams) (authuc.TokenPair, error)
	LoginInit(ctx context.Context, email, aHex string) (authuc.LoginInitResult, error)
	LoginFinalize(ctx context.Context, p authuc.LoginFinalizeParams) (authuc.LoginFinalizeResult, error)
	Refresh(ctx context.Context, refreshToken string) (authuc.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	ListDeviceSessions(ctx context.Context, userID uuid.UUID) ([]entity.DeviceSession, error)
	RevokeDeviceSession(ctx context.Context, userID, sessionID uuid.UUID) error
	RevokeOtherDeviceSessions(ctx context.Context, userID, currentSessionID uuid.UUID) error
}

// StorageService — бизнес-логика хранилища.
type StorageService interface {
	PresignPut(ctx context.Context, p storageuc.PresignPutParams) (*storageuc.PresignPutResult, error)
	PresignGet(ctx context.Context, userID, blobID uuid.UUID) (*storageuc.PresignGetResult, error)
	DeleteBlob(ctx context.Context, userID, blobID uuid.UUID) error
	ListBlobs(ctx context.Context, userID uuid.UUID) ([]entity.Blob, error)
	ListBlobsInFolder(ctx context.Context, userID uuid.UUID, folderID *uuid.UUID) ([]entity.Blob, error)
	MoveBlob(ctx context.Context, userID, blobID uuid.UUID, folderID *uuid.UUID) error
	CreateFolder(ctx context.Context, p storageuc.CreateFolderParams) (entity.Folder, error)
	GetFolder(ctx context.Context, userID, folderID uuid.UUID) (entity.Folder, error)
	ListFolders(ctx context.Context, userID uuid.UUID, parentID *uuid.UUID) ([]entity.Folder, error)
	RenameFolder(ctx context.Context, userID, folderID uuid.UUID, name string) (entity.Folder, error)
	MoveFolder(ctx context.Context, p storageuc.MoveFolderParams) error
	DeleteFolder(ctx context.Context, userID, folderID uuid.UUID) error
}

// SharingService — бизнес-логика шаринга файлов.
type SharingService interface {
	GetRecipientPublicKey(ctx context.Context, email string) ([]byte, error)
	CreateShare(ctx context.Context, p sharinguc.CreateShareParams) (entity.FileShareView, error)
	ListSharedWithMe(ctx context.Context, recipientID uuid.UUID) ([]entity.FileShareView, error)
	ListMyShares(ctx context.Context, blobID, ownerID uuid.UUID) ([]entity.FileShareView, error)
	GetSharedFile(ctx context.Context, shareID, callerID uuid.UUID) (sharinguc.SharedFileResult, error)
	RevokeShare(ctx context.Context, shareID, ownerID uuid.UUID) error
}

// Deps — зависимости v1-роутера; все поля — интерфейсы для тестируемости.
type Deps struct {
	Auth                 AuthService
	Tokens               restapi.ParseBearerJWT
	Sessions             restapi.SessionBlocklist
	Storage              StorageService
	Sharing              SharingService
	PublicKeyRateLimiter restapi.RateLimiter
	RefreshCookie        config.RefreshCookieConfig
	Logger               zerolog.Logger
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
		r.Use(restapi.AuthMiddleware(d.Tokens, d.Sessions, d.Logger))

		r.Route("/sessions", func(r chi.Router) {
			r.Get("/", listSessions(d))
			r.Delete("/", revokeOtherSessions(d))
			r.Delete("/{id}", revokeSession(d))
		})

		r.Route("/storage", func(r chi.Router) {
			r.Use(middleware.Timeout(30 * time.Minute))

			r.Post("/presign", storagePresignPut(d))
			r.Get("/blobs", storageListBlobs(d))
			r.Post("/blobs/{blobID}/presign-get", storagePresignGet(d))
			r.Delete("/blobs/{blobID}", storageDeleteBlob(d))
			r.Patch("/blobs/{blobID}/folder", moveBlob(d))
			r.Get("/blobs/{blobID}/shares", listMyShares(d))
			r.Post("/blobs/{blobID}/shares", createShare(d))

			r.Post("/folders", createFolder(d))
			r.Get("/folders", listFolders(d))
			r.Get("/folders/{folderID}", getFolder(d))
			r.Patch("/folders/{folderID}", renameFolder(d))
			r.Patch("/folders/{folderID}/move", moveFolder(d))
			r.Delete("/folders/{folderID}", deleteFolder(d))
		})

		r.Route("/shares", func(r chi.Router) {
			r.Get("/incoming", listSharedWithMe(d))
			r.Get("/{shareID}", getSharedFile(d))
			r.Delete("/{shareID}", revokeShare(d))
		})

		publicKeyRL := restapi.RateLimitMiddleware(d.PublicKeyRateLimiter, func(r *http.Request) string {
			uid, _ := restapi.UserIDFromContext(r.Context())
			return uid.String()
		})
		r.With(publicKeyRL).Get("/users/public-key", getRecipientPublicKey(d))
	})

	return r
}
