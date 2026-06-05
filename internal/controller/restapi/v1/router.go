package v1

import (
	"net"
	"net/http"
	"strings"
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
	Favorites            FavoritesService
	Audit                AuditService
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
		r.With(loginRL).Post("/register", register(d))
		r.With(loginRL).Post("/login/init", loginInit(d))
		r.With(loginRL).Post("/login/finalize", loginFinalize(d))
		r.With(loginRL).Post("/refresh", refresh(d))
		r.With(loginRL).Post("/logout", logout(d))

		r.With(loginRL).Post("/reset-password/request", requestPasswordReset(d))
		r.With(loginRL).Post("/reset-password/recovery-data", getRecoveryData(d))
		r.With(loginRL).Post("/reset-password/confirm", resetPasswordConfirm(d))
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
			r.Post("/blobs/initiate-multipart", storageInitiateMultipart(d))

			r.Get("/blobs", storageListBlobs(d))
			r.Post("/blobs/{blobID}/complete-multipart", storageCompleteMultipart(d))
			r.Post("/blobs/{blobID}/presign-get", storagePresignGet(d))
			r.Delete("/blobs/{blobID}/abort", storageAbortUpload(d))
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

		r.Get("/auth/crypto-salt", getCryptoSalt(d))
		r.With(publicKeyRL).Get("/users/public-key", getRecipientPublicKey(d))

		r.Get("/audit", listAuditEvents(d))

		r.Route("/favorites", func(r chi.Router) {
			r.Get("/", favoritesList(d))
			r.Post("/blobs/{id}", favoritesAdd(d, "blob"))
			r.Delete("/blobs/{id}", favoritesRemove(d, "blob"))
			r.Post("/folders/{id}", favoritesAdd(d, "folder"))
			r.Delete("/folders/{id}", favoritesRemove(d, "folder"))
		})
	})

	return r
}

func userIDKey(r *http.Request) string {
	uid, _ := restapi.UserIDFromContext(r.Context())
	return uid.String()
}

func realIP(r *http.Request) string {
	addr := r.RemoteAddr
	// Strip CIDR suffix (e.g. "172.19.0.1/32" → "172.19.0.1") that some
	// Docker network configurations append to the remote address.
	if i := strings.IndexByte(addr, '/'); i != -1 {
		addr = addr[:i]
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
