package config

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr string

	DatabaseURL string

	RedisURL string

	LogLevel       string
	FrontendOrigin string

	StorageQuotaBytes int64
	MaxUploadBytes    int64
	TrashTTL          time.Duration

	Server        ServerConfig
	JWT           JWTConfig
	RefreshCookie RefreshCookieConfig
	MinIO         MinIOConfig
	SMTP          SMTPConfig
}

type ServerConfig struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type JWTConfig struct {
	Secret     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type RefreshCookieConfig struct {
	Name     string
	Path     string
	Domain   string
	Secure   bool
	SameSite http.SameSite
}

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

type SMTPConfig struct {
	ResendAPIKey string
	From         string
}

func Load() (Config, error) {
	l := &loader{}
	cfg := l.build()
	if len(l.errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(l.errs, "\n  - "))
	}
	return cfg, nil
}

type loader struct {
	errs []string
}

func (l *loader) build() Config {
	return Config{
		HTTPAddr:          envStr("HTTP_ADDR", ":8080"),
		DatabaseURL:       l.requireStr("DATABASE_URL"),
		RedisURL:          l.requireStr("REDIS_URL"),
		LogLevel:          envStr("LOG_LEVEL", "info"),
		FrontendOrigin:    envStr("FRONTEND_ORIGIN", ""),
		StorageQuotaBytes: envInt64("STORAGE_QUOTA_BYTES", 0),
		MaxUploadBytes:    envInt64("MAX_UPLOAD_BYTES", 0),
		TrashTTL:          l.requirePosDuration("TRASH_TTL", 30*24*time.Hour),
		Server:            l.buildServer(),
		JWT:               l.buildJWT(),
		RefreshCookie:     l.buildRefreshCookie(),
		MinIO:             l.buildMinIO(),
		SMTP:              l.buildSMTP(),
	}
}

func (l *loader) buildServer() ServerConfig {
	return ServerConfig{
		ReadHeaderTimeout: l.requirePosDuration("HTTP_READ_HEADER_TIMEOUT", 10*time.Second),
		ReadTimeout:       l.requirePosDuration("HTTP_READ_TIMEOUT", 35*time.Minute),
		WriteTimeout:      l.requirePosDuration("HTTP_WRITE_TIMEOUT", 35*time.Minute),
		IdleTimeout:       l.requirePosDuration("HTTP_IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout:   l.requirePosDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
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
	sameSite, err := parseSameSite(envStr("REFRESH_COOKIE_SAMESITE", "strict"))
	if err != nil {
		l.errs = append(l.errs, "REFRESH_COOKIE_SAMESITE: "+err.Error())
		sameSite = http.SameSiteLaxMode
	}
	secure := envBool("REFRESH_COOKIE_SECURE", false)
	if sameSite == http.SameSiteNoneMode && !secure {
		l.errs = append(l.errs, "REFRESH_COOKIE_SAMESITE=none requires REFRESH_COOKIE_SECURE=true")
	}

	// Use __Host- prefix in secure mode: forces Secure, Path=/, no Domain (per RFC 6265bis).
	// In dev (Secure=false) browsers reject __Host- cookies, so fall back to a plain name.
	defaultName := "refresh_token"
	if secure {
		defaultName = "__Host-refresh_token"
	}
	name := envStr("REFRESH_COOKIE_NAME", defaultName)
	path := envStr("REFRESH_COOKIE_PATH", "/")
	domain := envStr("REFRESH_COOKIE_DOMAIN", "")

	if strings.HasPrefix(name, "__Host-") {
		if !secure {
			l.errs = append(l.errs, "REFRESH_COOKIE_NAME: __Host- prefix requires REFRESH_COOKIE_SECURE=true")
		}
		if domain != "" {
			l.errs = append(l.errs, "REFRESH_COOKIE_NAME: __Host- prefix requires REFRESH_COOKIE_DOMAIN to be empty")
		}
		if path != "/" {
			l.errs = append(l.errs, "REFRESH_COOKIE_NAME: __Host- prefix requires REFRESH_COOKIE_PATH=/")
		}
	}

	return RefreshCookieConfig{
		Name:     name,
		Path:     path,
		Domain:   domain,
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

func (l *loader) buildSMTP() SMTPConfig {
	return SMTPConfig{
		ResendAPIKey: envStr("RESEND_API_KEY", ""),
		From:         envStr("SMTP_FROM", ""),
	}
}

func (l *loader) requireStr(key string) string {
	v := envStr(key, "")
	if v == "" {
		l.errs = append(l.errs, key+": required")
	}
	return v
}

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

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envInt64(key string, def int64) int64 {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

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
