// Package config — конфигурация из env (twelve-factor), как в go-clean-template.
package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	HTTPAddr string

	DatabaseURL string
	DBInit      bool
	MigrateOnly bool

	LogLevel string

	JWT    JWTConfig
	Opaque OpaqueConfig

	// MinIO — объектное хранилище (пустой Endpoint = API загрузки не подключается).
	MinIO MinIOConfig
}

type JWTConfig struct {
	Secret string

	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type OpaqueConfig struct {
	KeysFile string

	// Optional identity used inside AKE transcript.
	// If empty, server identity is derived inside the library from server public key.
	ServerIdentityHex string
}

// MinIOConfig — S3-совместимое API (MinIO / AWS S3).
type MinIOConfig struct {
	Endpoint       string
	PublicEndpoint string
	AccessKey      string
	SecretKey      string
	Bucket         string
	UseSSL         bool
	Region         string

	PresignTTL time.Duration
}

// Load читает конфигурацию из переменных окружения и возвращает ошибку, если обязательные значения отсутствуют.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:    envString("HTTP_ADDR", ":8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		DBInit:      envBool("DB_INIT", false),
		MigrateOnly: envBool("MIGRATE_ONLY", false),
		LogLevel:    envString("LOG_LEVEL", "info"),
		JWT: JWTConfig{
			Secret:     envString("JWT_SECRET", ""),
			AccessTTL:  envDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL: envDuration("JWT_REFRESH_TTL", 30*24*time.Hour),
		},
		Opaque: OpaqueConfig{
			KeysFile:          envString("OPAQUE_KEYS_FILE", "./data/opaque_server_keys.json"),
			ServerIdentityHex: envString("OPAQUE_SERVER_ID_HEX", ""),
		},
		MinIO: MinIOConfig{
			Endpoint:       envString("MINIO_ENDPOINT", ""),
			PublicEndpoint: envString("MINIO_PUBLIC_ENDPOINT", ""),
			AccessKey:      envString("MINIO_ACCESS_KEY", ""),
			SecretKey:      envString("MINIO_SECRET_KEY", ""),
			Bucket:         envString("MINIO_BUCKET", "blobs"),
			UseSSL:         envBool("MINIO_USE_SSL", false),
			Region:         envString("MINIO_REGION", "us-east-1"),
			PresignTTL:     envDuration("MINIO_PRESIGN_TTL", time.Hour),
		},
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWT.Secret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}

	// Make KeysFile absolute to avoid surprises when running from another directory.
	if cfg.Opaque.KeysFile != "" && !filepath.IsAbs(cfg.Opaque.KeysFile) {
		if wd, err := os.Getwd(); err == nil {
			cfg.Opaque.KeysFile = filepath.Join(wd, cfg.Opaque.KeysFile)
		}
	}

	// Validate optional identity if provided.
	if cfg.Opaque.ServerIdentityHex != "" {
		if _, err := hex.DecodeString(cfg.Opaque.ServerIdentityHex); err != nil {
			return Config{}, fmt.Errorf("OPAQUE_SERVER_ID_HEX must be hex: %w", err)
		}
	}

	if cfg.MinIO.Endpoint != "" {
		if cfg.MinIO.AccessKey == "" || cfg.MinIO.SecretKey == "" {
			return Config{}, fmt.Errorf("MINIO_ACCESS_KEY and MINIO_SECRET_KEY are required when MINIO_ENDPOINT is set")
		}
	}

	return cfg, nil
}

func envString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		switch v {
		case "1", "true", "TRUE", "True":
			return true
		case "0", "false", "FALSE", "False":
			return false
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
