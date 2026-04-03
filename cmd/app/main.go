package main

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"cloud-backend/config"
	"cloud-backend/internal/app"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := log.With().Str("service", "cloud-backend").Logger()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal().Err(err).Msg("config error")
	}

	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		logger.Fatal().Err(err).Str("level", cfg.LogLevel).Msg("invalid LOG_LEVEL")
	}
	zerolog.SetGlobalLevel(logLevel)

	if err := app.Run(cfg, logger); err != nil {
		logger.Fatal().Err(err).Msg("app stopped")
	}
}
