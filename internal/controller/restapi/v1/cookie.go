package v1

import (
	"net/http"
	"strings"

	"cloud-backend/config"
)

const deviceCookieMaxAge = 90 * 24 * 60 * 60

func deviceCookieName(cfg config.RefreshCookieConfig) string {
	if strings.HasPrefix(cfg.Name, "__Host-") {
		return "__Host-device_id"
	}
	return "device_id"
}

func readDeviceCookie(r *http.Request, cfg config.RefreshCookieConfig) string {
	c, err := r.Cookie(deviceCookieName(cfg))
	if err != nil || c.Value == "" {
		return ""
	}
	return c.Value
}

func setDeviceCookie(w http.ResponseWriter, cfg config.RefreshCookieConfig, deviceID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     deviceCookieName(cfg),
		Value:    deviceID,
		Path:     cfg.Path,
		Domain:   cfg.Domain,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
		MaxAge:   deviceCookieMaxAge,
	})
}

func readRefreshToken(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil || c.Value == "" {
		return ""
	}
	return c.Value
}

func setRefreshTokenCookie(w http.ResponseWriter, cfg config.RefreshCookieConfig, token string, maxAgeSec int) {
	c := baseRefreshCookie(cfg)
	c.Value = token
	c.MaxAge = maxAgeSec
	http.SetCookie(w, &c)
}

func clearRefreshTokenCookie(w http.ResponseWriter, cfg config.RefreshCookieConfig) {
	c := baseRefreshCookie(cfg)
	c.Value = ""
	c.MaxAge = -1
	http.SetCookie(w, &c)
}

func baseRefreshCookie(cfg config.RefreshCookieConfig) http.Cookie {
	return http.Cookie{
		Name:     cfg.Name,
		Path:     cfg.Path,
		Domain:   cfg.Domain,
		HttpOnly: true,
		Secure:   cfg.Secure,
		SameSite: cfg.SameSite,
	}
}
