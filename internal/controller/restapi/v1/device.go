package v1

import (
	"net/http"
	"strings"

	authuc "cloud-backend/internal/usecase/auth"
	"cloud-backend/pkg/useragent"
)

const maxUserAgentLen = 512

// parseDeviceInfo извлекает данные об устройстве из HTTP-запроса.
func parseDeviceInfo(r *http.Request) authuc.DeviceInfo {
	ua := r.Header.Get("User-Agent")
	if len(ua) > maxUserAgentLen {
		ua = ua[:maxUserAgentLen]
	}
	return authuc.DeviceInfo{
		UserAgent:  ua,
		IPAddress:  clientIP(r),
		DeviceName: useragent.Parse(ua),
	}
}

// clientIP extracts the best-effort client address from the request.
// X-Forwarded-For / X-Real-IP are used for display only — they are not
// trusted for security decisions.
func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx > 0 {
		return ip[:idx]
	}
	return ip
}
