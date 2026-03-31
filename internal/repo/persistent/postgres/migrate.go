package postgres

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog"

	appmigrations "cloud-backend/migrations"
)

// RunMigrations применяет все неприменённые миграции из папки /migrations (встроены в бинарник).
func RunMigrations(pool *pgxpool.Pool, log zerolog.Logger) error {
	db := stdlib.OpenDBFromPool(pool)
	defer func() {
		if err := db.Close(); err != nil {
			log.Warn().Err(err).Msg("migrations: db close failed")
		}
	}()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("migrate ping: %w", err)
	}

	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("migrate pgx driver: %w", err)
	}

	src, err := iofs.New(appmigrations.Files, ".")
	if err != nil {
		return fmt.Errorf("migrate iofs: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrate new: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Warn().Err(srcErr).Msg("migrations: source close failed")
		}
		if dbErr != nil {
			log.Warn().Err(dbErr).Msg("migrations: driver close failed")
		}
	}()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
