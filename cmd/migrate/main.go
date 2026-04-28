package main

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"cloud-backend/internal/repo/persistent/postgres"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := log.With().Str("service", "migrate").Logger()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		logger.Fatal().Msg("DATABASE_URL is required")
	}

	pool, err := postgres.NewPool(dbURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("db connect failed")
	}
	defer pool.Close()

	if err := postgres.RunMigrations(pool, logger); err != nil {
		logger.Fatal().Err(err).Msg("migrations failed")
	}
	logger.Info().Msg("migrations applied successfully")
}
