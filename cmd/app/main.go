package main

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"cloud-backend/config"
	"cloud-backend/internal/app"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config error")
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logLevel, parseErr := zerolog.ParseLevel(cfg.LogLevel)
	if parseErr != nil {
		logLevel = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevel)
	logger := log.With().Str("service", "cloud-backend").Logger()

	if err := app.Run(cfg, logger); err != nil {
		logger.Fatal().Err(err).Msg("app stopped")
	}
}
