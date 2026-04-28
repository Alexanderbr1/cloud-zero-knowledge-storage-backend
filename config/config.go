// Package config — конфигурация приложения из переменных окружения (twelve-factor).
package config

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config — конфигурация приложения.
type Config struct {
	HTTPAddr string

	DatabaseURL string

	RedisURL string

	LogLevel string

	Server        ServerConfig
	JWT           JWTConfig
	RefreshCookie RefreshCookieConfig
	MinIO         MinIOConfig
}

// ServerConfig — таймауты HTTP-сервера.
type ServerConfig struct {
	ReadTimeout     time.Duration // HTTP_READ_TIMEOUT (default: 35m — даёт запас для крупных ответов)
	WriteTimeout    time.Duration // HTTP_WRITE_TIMEOUT (default: 35m)
	IdleTimeout     time.Duration // HTTP_IDLE_TIMEOUT (default: 120s)
	ShutdownTimeout time.Duration // HTTP_SHUTDOWN_TIMEOUT (default: 10s)
}

// JWTConfig — параметры JWT и refresh-токенов.
type JWTConfig struct {
	// Secret — ключ подписи HS256. Минимум 32 символа (256 бит).
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// RefreshCookieConfig — атрибуты HttpOnly-куки с refresh-токеном.
type RefreshCookieConfig struct {
	Name     string
	Path     string
	Domain   string
	Secure   bool
	SameSite http.SameSite
}

// MinIOConfig — S3-совместимое объектное хранилище.
type MinIOConfig struct {
	Endpoint       string
	PublicEndpoint string
	AccessKey      string
	SecretKey      string
	Bucket         string
	UseSSL         bool
	Region         string
	PresignTTL     time.Duration
}

// Load читает конфигурацию из env и возвращает сразу все ошибки валидации.
func Load() (Config, error) {
	l := &loader{}
	cfg := l.build()
	if len(l.errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(l.errs, "\n  - "))
	}
	return cfg, nil
}

// loader накапливает ошибки конфигурации и не останавливается на первой.
type loader struct {
	errs []string
}

func (l *loader) build() Config {
	return Config{
		HTTPAddr:      envStr("HTTP_ADDR", ":8080"),
		DatabaseURL:   l.requireStr("DATABASE_URL"),
		RedisURL:      l.requireStr("REDIS_URL"),
		LogLevel:      envStr("LOG_LEVEL", "info"),
		Server:        l.buildServer(),
		JWT:           l.buildJWT(),
		RefreshCookie: l.buildRefreshCookie(),
		MinIO:         l.buildMinIO(),
	}
}

func (l *loader) buildServer() ServerConfig {
	return ServerConfig{
		ReadTimeout:     l.requirePosDuration("HTTP_READ_TIMEOUT", 35*time.Minute),
		WriteTimeout:    l.requirePosDuration("HTTP_WRITE_TIMEOUT", 35*time.Minute),
		IdleTimeout:     l.requirePosDuration("HTTP_IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout: l.requirePosDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
	}
}

func (l *loader) buildJWT() JWTConfig {
	secret := l.requireStr("JWT_SECRET")
	if secret != "" && len(secret) < 32 {
		l.errs = append(l.errs, "JWT_SECRET: must be at least 32 characters (HS256 requires 256-bit key)")
	}
	return JWTConfig{
		Secret:     secret,
		AccessTTL:  l.requirePosDuration("JWT_ACCESS_TTL", 15*time.Minute),
		RefreshTTL: l.requirePosDuration("JWT_REFRESH_TTL", 30*24*time.Hour),
	}
}

func (l *loader) buildRefreshCookie() RefreshCookieConfig {
	sameSite, err := parseSameSite(envStr("REFRESH_COOKIE_SAMESITE", "lax"))
	if err != nil {
		l.errs = append(l.errs, "REFRESH_COOKIE_SAMESITE: "+err.Error())
		sameSite = http.SameSiteLaxMode
	}
	secure := envBool("REFRESH_COOKIE_SECURE", false)
	if sameSite == http.SameSiteNoneMode && !secure {
		l.errs = append(l.errs, "REFRESH_COOKIE_SAMESITE=none requires REFRESH_COOKIE_SECURE=true")
	}
	return RefreshCookieConfig{
		Name:     envStr("REFRESH_COOKIE_NAME", "refresh_token"),
		Path:     envStr("REFRESH_COOKIE_PATH", "/"),
		Domain:   envStr("REFRESH_COOKIE_DOMAIN", ""),
		Secure:   secure,
		SameSite: sameSite,
	}
}

func (l *loader) buildMinIO() MinIOConfig {
	return MinIOConfig{
		Endpoint:       l.requireStr("MINIO_ENDPOINT"),
		PublicEndpoint: envStr("MINIO_PUBLIC_ENDPOINT", ""),
		AccessKey:      l.requireStr("MINIO_ACCESS_KEY"),
		SecretKey:      l.requireStr("MINIO_SECRET_KEY"),
		Bucket:         envStr("MINIO_BUCKET", "blobs"),
		UseSSL:         envBool("MINIO_USE_SSL", false),
		Region:         envStr("MINIO_REGION", "us-east-1"),
		PresignTTL:     l.requirePosDuration("MINIO_PRESIGN_TTL", time.Hour),
	}
}

// requireStr добавляет ошибку, если переменная не задана или пуста.
func (l *loader) requireStr(key string) string {
	v := envStr(key, "")
	if v == "" {
		l.errs = append(l.errs, key+": required")
	}
	return v
}

// requirePosDuration возвращает duration ≥ 0 или добавляет ошибку при невалидном / неположительном значении.
func (l *loader) requirePosDuration(key string, def time.Duration) time.Duration {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		l.errs = append(l.errs, fmt.Sprintf("%s: invalid duration %q: %v", key, raw, err))
		return def
	}
	if d <= 0 {
		l.errs = append(l.errs, fmt.Sprintf("%s: must be positive, got %q", key, raw))
		return def
	}
	return d
}

// envStr читает строковую переменную окружения или возвращает def.
func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// envBool читает булеву переменную окружения (1/0, true/false, t/f и т.д.).
// При невалидном значении молча возвращает def — булевы флаги не критичны.
func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// parseSameSite преобразует строку в http.SameSite и возвращает ошибку для неизвестных значений.
func parseSameSite(s string) (http.SameSite, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "strict":
		return http.SameSiteStrictMode, nil
	case "lax":
		return http.SameSiteLaxMode, nil
	case "none":
		return http.SameSiteNoneMode, nil
	default:
		return http.SameSiteLaxMode, fmt.Errorf("unknown value %q; valid values: strict, lax, none", s)
	}
}
