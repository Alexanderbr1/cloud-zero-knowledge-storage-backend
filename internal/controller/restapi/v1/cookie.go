package v1

import (
	"net/http"

	"cloud-backend/config"
)

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
