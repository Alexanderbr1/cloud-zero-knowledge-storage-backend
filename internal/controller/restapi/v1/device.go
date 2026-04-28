package v1

import (
	"net/http"
	"strings"

	authuc "cloud-backend/internal/usecase/auth"
	"cloud-backend/pkg/useragent"
)

const maxUserAgentLen = 512

// parseDeviceInfo извлекает данные об устройстве из HTTP-запроса.
// IP берётся из X-Forwarded-For / X-Real-IP только для отображения —
// может быть подделан клиентом и не используется для решений безопасности.
func parseDeviceInfo(r *http.Request) authuc.DeviceInfo {
	ua := r.Header.Get("User-Agent")
	if len(ua) > maxUserAgentLen {
		ua = ua[:maxUserAgentLen]
	}

	ip := r.RemoteAddr
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		ip = strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	} else if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		ip = xrip
	} else if idx := strings.LastIndex(ip, ":"); idx > 0 {
		ip = ip[:idx]
	}

	return authuc.DeviceInfo{
		UserAgent:  ua,
		IPAddress:  ip,
		DeviceName: useragent.Parse(ua),
	}
}
