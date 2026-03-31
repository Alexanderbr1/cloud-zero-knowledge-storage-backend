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

	"github.com/rs/zerolog"

	"cloud-backend/config"
	"cloud-backend/internal/controller/restapi"
	v1 "cloud-backend/internal/controller/restapi/v1"
	"cloud-backend/internal/repo/persistent/postgres"
	miniostore "cloud-backend/internal/repo/storage/minio"
	authuc "cloud-backend/internal/usecase/auth"
	storageuc "cloud-backend/internal/usecase/storage"
	jwtpkg "cloud-backend/pkg/jwt"
)

var _ restapi.ParseBearerJWT = (*jwtpkg.Service)(nil)

// Run — composition root: БД, миграции, use case-ы, HTTP (см. internal/app в go-clean-template).
func Run(cfg config.Config, log zerolog.Logger) error {
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

	store := postgres.NewStorage(pool)
	tokens := jwtpkg.NewService([]byte(cfg.JWT.Secret), cfg.JWT.AccessTTL)
	authSvc := &authuc.Service{
		Users:      store,
		Sessions:   store,
		Tokens:     tokens,
		RefreshTTL: cfg.JWT.RefreshTTL,
	}

	initCtx := context.Background()
	var storageSvc *storageuc.Service
	if cfg.MinIO.Endpoint != "" {
		ms, err := miniostore.NewStore(cfg.MinIO)
		if err != nil {
			return fmt.Errorf("minio: %w", err)
		}
		if err := ms.EnsureBucket(initCtx); err != nil {
			return fmt.Errorf("minio bucket: %w", err)
		}
		storageSvc = &storageuc.Service{
			Objects:    ms,
			Blobs:      store,
			PresignTTL: cfg.MinIO.PresignTTL,
		}
	}

	handler := newHTTPHandler(v1.Deps{
		Auth:    authSvc,
		Tokens:  tokens,
		Storage: storageSvc,
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      handler,
		ReadTimeout:  35 * time.Minute,
		WriteTimeout: 35 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info().Str("addr", cfg.HTTPAddr).Msg("listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		log.Info().Msg("shutdown requested")
		shCtx, shCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shCancel()
		_ = srv.Shutdown(shCtx)
		log.Info().Msg("shutdown complete")
	}
	return nil
}
