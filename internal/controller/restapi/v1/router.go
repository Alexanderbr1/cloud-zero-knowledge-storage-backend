package v1

import (
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"cloud-backend/config"
	"cloud-backend/internal/controller/restapi"
)

type Deps struct {
	Auth                 AuthService
	DeviceSessions       SessionsService
	Tokens               restapi.ParseBearerJWT
	Blocklist            restapi.SessionBlocklist
	Blobs                BlobService
	Folders              FolderService
	Trash                TrashService
	Sharing              SharingService
	PublicKeyRateLimiter restapi.RateLimiter
	LoginRateLimiter     restapi.RateLimiter
	RefreshCookie        config.RefreshCookieConfig
	Logger               zerolog.Logger
}

func NewRouter(d Deps) chi.Router {
	loginRL := restapi.RateLimitMiddleware(d.LoginRateLimiter, realIP)
	publicKeyRL := restapi.RateLimitMiddleware(d.PublicKeyRateLimiter, userIDKey)

	r := chi.NewRouter()

	r.Get("/health", health())

	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", register(d))
		r.With(loginRL).Post("/login/init", loginInit(d))
		r.With(loginRL).Post("/login/finalize", loginFinalize(d))
		r.Post("/refresh", refresh(d))
		r.Post("/logout", logout(d))
	})

	r.Group(func(r chi.Router) {
		r.Use(restapi.AuthMiddleware(d.Tokens, d.Blocklist, d.Logger))

		r.Route("/sessions", func(r chi.Router) {
			r.Get("/", listSessions(d))
			r.Delete("/", revokeOtherSessions(d))
			r.Delete("/{id}", revokeSession(d))
		})

		r.Route("/storage", func(r chi.Router) {
			r.Use(middleware.Timeout(30 * time.Minute))

			r.Get("/usage", storageGetUsage(d))
			r.Post("/presign", storagePresignPut(d))

			r.Get("/blobs", storageListBlobs(d))
			r.Post("/blobs/{blobID}/confirm-upload", storageConfirmUpload(d))
			r.Post("/blobs/{blobID}/presign-get", storagePresignGet(d))
			r.Delete("/blobs/{blobID}", storageDeleteBlob(d))
			r.Patch("/blobs/{blobID}", renameBlob(d))
			r.Patch("/blobs/{blobID}/folder", moveBlob(d))
			r.Get("/blobs/{blobID}/shares", listMyShares(d))
			r.Post("/blobs/{blobID}/shares", createShare(d))

			r.Get("/search", storageSearch(d))

			r.Post("/folders", createFolder(d))
			r.Get("/folders", listFolders(d))
			r.Get("/folders/{folderID}", getFolder(d))
			r.Patch("/folders/{folderID}", renameFolder(d))
			r.Patch("/folders/{folderID}/move", moveFolder(d))
			r.Delete("/folders/{folderID}", deleteFolder(d))
		})

		r.Route("/trash", func(r chi.Router) {
			r.Get("/", trashList(d))
			r.Delete("/", trashEmpty(d))
			r.Post("/blobs/{blobID}/restore", trashRestoreBlob(d))
			r.Delete("/blobs/{blobID}", trashDeleteBlob(d))
			r.Post("/folders/{folderID}/restore", trashRestoreFolder(d))
			r.Delete("/folders/{folderID}", trashDeleteFolder(d))
		})

		r.Route("/shares", func(r chi.Router) {
			r.Get("/incoming", listSharedWithMe(d))
			r.Get("/{shareID}", getSharedFile(d))
			r.Delete("/{shareID}", revokeShare(d))
		})

		r.With(publicKeyRL).Get("/users/public-key", getRecipientPublicKey(d))
	})

	return r
}

func userIDKey(r *http.Request) string {
	uid, _ := restapi.UserIDFromContext(r.Context())
	return uid.String()
}

func realIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
