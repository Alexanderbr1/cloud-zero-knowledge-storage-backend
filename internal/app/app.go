package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"cloud-backend/config"
	v1 "cloud-backend/internal/controller/restapi/v1"
	rediscache "cloud-backend/internal/repo/cache/redis"
	"cloud-backend/internal/repo/persistent/postgres"
	miniostore "cloud-backend/internal/repo/storage/minio"
	audituc "cloud-backend/internal/usecase/audit"
	authuc "cloud-backend/internal/usecase/auth"
	favoritesuc "cloud-backend/internal/usecase/favorites"
	sharinguc "cloud-backend/internal/usecase/sharing"
	storageuc "cloud-backend/internal/usecase/storage"
	jwtpkg "cloud-backend/pkg/jwt"
	mailerpkg "cloud-backend/pkg/mailer"
)

type wiredDeps struct {
	router   v1.Deps
	sessions interface{ CleanOrphanedSessions(context.Context) error }
	trash    interface {
		CleanExpiredTrash(context.Context, time.Duration) error
	}
	orphans interface {
		CleanOrphanedBlobs(context.Context, time.Duration) error
	}
}

func Run(cfg config.Config, log zerolog.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := postgres.NewPool(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()

	redisClient, closeRedis, err := rediscache.NewClient(ctx, cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	defer closeRedis()
	log.Info().Str("url", cfg.RedisURL).Msg("redis connected")

	ms, err := miniostore.NewStore(miniostore.StoreConfig{
		Endpoint:       cfg.MinIO.Endpoint,
		PublicEndpoint: cfg.MinIO.PublicEndpoint,
		AccessKey:      cfg.MinIO.AccessKey,
		SecretKey:      cfg.MinIO.SecretKey,
		Bucket:         cfg.MinIO.Bucket,
		UseSSL:         cfg.MinIO.UseSSL,
		Region:         cfg.MinIO.Region,
	})
	if err != nil {
		return fmt.Errorf("minio: %w", err)
	}
	if err := ms.EnsureBucket(ctx); err != nil {
		return fmt.Errorf("minio bucket: %w", err)
	}

	wired, err := wireDeps(ctx, cfg, log, pool, redisClient, ms)
	if err != nil {
		return err
	}

	go runCleanupJob(ctx, time.Hour, 30*time.Second, "session cleanup", wired.sessions.CleanOrphanedSessions, log)
	go runCleanupJob(ctx, time.Hour, 60*time.Second, "trash cleanup", func(c context.Context) error {
		return wired.trash.CleanExpiredTrash(c, cfg.TrashTTL)
	}, log)
	go runCleanupJob(ctx, time.Hour, 60*time.Second, "orphan blobs cleanup", func(c context.Context) error {
		return wired.orphans.CleanOrphanedBlobs(c, cfg.MinIO.PresignTTL)
	}, log)

	return serve(ctx, newHTTPServer(cfg, wired.router, log), log, cfg.Server.ShutdownTimeout)
}

func wireDeps(
	ctx context.Context,
	cfg config.Config,
	log zerolog.Logger,
	pool *pgxpool.Pool,
	redisClient *goredis.Client,
	ms *miniostore.Store,
) (wiredDeps, error) {
	store := postgres.NewStorage(pool)
	tokens := jwtpkg.NewService([]byte(cfg.JWT.Secret), cfg.JWT.AccessTTL)

	blocklist  := rediscache.NewSessionBlocklist(redisClient)
	publicKeyRL := rediscache.NewRateLimiter(redisClient, "rl:pubkey:", 20, time.Minute)
	loginRL     := rediscache.NewRateLimiter(redisClient, "rl:login:", 10, time.Minute)

	mailer := mailerpkg.New(mailerpkg.Config{
		ResendAPIKey: cfg.SMTP.ResendAPIKey,
		From:         cfg.SMTP.From,
	})

	auditSvc := &audituc.Service{
		Repo:   store,
		Logger: log,
	}

	authSvc := &authuc.Service{
		Users:          store,
		Sessions:       store,
		DeviceSessions: store,
		Tokens:         tokens,
		Blocklist:      blocklist,
		AccessTTL:      cfg.JWT.AccessTTL,
		RefreshTTL:     cfg.JWT.RefreshTTL,
		SRPSessions:    rediscache.NewSRPSessionStore(redisClient),
		Logger:         log,
		Notifier:       mailer,
		Audit:          auditSvc,
	}

	storageSvc := &storageuc.Service{
		Objects:        ms,
		Blobs:          store,
		BlobTrash:      store,
		Folders:        store,
		FolderTrash:    store,
		PresignTTL:     cfg.MinIO.PresignTTL,
		MaxUploadBytes: cfg.MaxUploadBytes,
		QuotaBytes:     cfg.StorageQuotaBytes,
	}

	sharingSvc := &sharinguc.Service{
		Shares:     store,
		Users:      store,
		Blobs:      store,
		Objects:    ms,
		PresignTTL: cfg.MinIO.PresignTTL,
		Notifier:   mailer,
		Logger:     log,
	}

	favoritesSvc := &favoritesuc.Service{
		Repo: store,
	}

	return wiredDeps{
		router: v1.Deps{
			Auth:                 authSvc,
			DeviceSessions:       authSvc,
			Tokens:               tokens,
			Blocklist:            blocklist,
			Blobs:                storageSvc,
			Folders:              storageSvc,
			Trash:                storageSvc,
			Sharing:              sharingSvc,
			Favorites:            favoritesSvc,
			Audit:                auditSvc,
			PublicKeyRateLimiter: publicKeyRL,
			LoginRateLimiter:     loginRL,
			RefreshCookie:        cfg.RefreshCookie,
			Logger:               log,
		},
		sessions: authSvc,
		trash:    storageSvc,
		orphans:  storageSvc,
	}, nil
}

func newHTTPServer(cfg config.Config, deps v1.Deps, log zerolog.Logger) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           newHTTPHandler(deps, log),
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
		ReadTimeout:       cfg.Server.ReadTimeout,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}
}

func runCleanupJob(ctx context.Context, interval, timeout time.Duration, name string, fn func(context.Context) error, log zerolog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanCtx, cancel := context.WithTimeout(ctx, timeout)
			if err := fn(cleanCtx); err != nil {
				log.Error().Err(err).Msgf("%s failed", name)
			}
			cancel()
		}
	}
}

func serve(ctx context.Context, srv *http.Server, log zerolog.Logger, shutdownTimeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("listening")
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		return err
	case sig := <-sigCh:
		log.Info().Str("signal", sig.String()).Msg("shutdown requested")
		shCtx, shCancel := context.WithTimeout(ctx, shutdownTimeout)
		defer shCancel()
		if err := srv.Shutdown(shCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		log.Info().Msg("shutdown complete")
	}
	return nil
}
