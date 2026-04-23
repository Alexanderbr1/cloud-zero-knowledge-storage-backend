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
	"github.com/rs/zerolog"

	"cloud-backend/config"
	v1 "cloud-backend/internal/controller/restapi/v1"
	"cloud-backend/internal/repo/persistent/postgres"
	miniostore "cloud-backend/internal/repo/storage/minio"
	authuc "cloud-backend/internal/usecase/auth"
	storageuc "cloud-backend/internal/usecase/storage"
	jwtpkg "cloud-backend/pkg/jwt"
)

func Run(cfg config.Config, log zerolog.Logger) error {
	ctx := context.Background()

	pool, err := postgres.NewPool(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer pool.Close()

	if cfg.DBInit || cfg.MigrateOnly {
		if err := postgres.RunMigrations(pool, log); err != nil {
			return fmt.Errorf("migrations: %w", err)
		}
	}
	if cfg.MigrateOnly {
		log.Info().Msg("migrations applied; exiting (MIGRATE_ONLY=true)")
		return nil
	}

	deps, err := wireDeps(ctx, cfg, log, pool)
	if err != nil {
		return err
	}

	go runSessionCleanupJob(ctx, deps.Auth.(*authuc.Service), log)

	return serve(ctx, newHTTPServer(cfg, deps), log, cfg.Server.ShutdownTimeout)
}

// wireDeps строит граф зависимостей приложения поверх уже открытого пула БД.
func wireDeps(ctx context.Context, cfg config.Config, log zerolog.Logger, pool *pgxpool.Pool) (v1.Deps, error) {
	store := postgres.NewStorage(pool)
	tokens := jwtpkg.NewService([]byte(cfg.JWT.Secret), cfg.JWT.AccessTTL)

	authSvc := &authuc.Service{
		Users:          store,
		Sessions:       store,
		DeviceSessions: store,
		Tokens:         tokens,
		RefreshTTL:     cfg.JWT.RefreshTTL,
		SRPSessions:    authuc.NewSRPSessionStore(ctx),
	}

	ms, err := miniostore.NewStore(cfg.MinIO)
	if err != nil {
		return v1.Deps{}, fmt.Errorf("minio: %w", err)
	}
	if err := ms.EnsureBucket(ctx); err != nil {
		return v1.Deps{}, fmt.Errorf("minio bucket: %w", err)
	}
	storageSvc := &storageuc.Service{
		Objects:    ms,
		Blobs:      store,
		PresignTTL: cfg.MinIO.PresignTTL,
	}

	return v1.Deps{
		Auth:          authSvc,
		Tokens:        tokens,
		Storage:       storageSvc,
		RefreshCookie: cfg.RefreshCookie,
	}, nil
}

func newHTTPServer(cfg config.Config, deps v1.Deps) *http.Server {
	return &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      newHTTPHandler(deps),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}
}

func runSessionCleanupJob(ctx context.Context, svc *authuc.Service, log zerolog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			if err := svc.CleanOrphanedSessions(cleanCtx); err != nil {
				log.Error().Err(err).Msg("session cleanup job failed")
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
