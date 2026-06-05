package v1

import (
	"net/http"

	"github.com/google/uuid"

	"cloud-backend/config"
	authuc "cloud-backend/internal/usecase/auth"
	"cloud-backend/pkg/useragent"
)

const maxUserAgentLen = 512

func ensureDeviceID(r *http.Request, cfg config.RefreshCookieConfig) string {
	if id := readDeviceCookie(r, cfg); id != "" {
		return id
	}
	return uuid.New().String()
}

func parseDeviceInfo(r *http.Request, deviceID string) authuc.DeviceInfo {
	ua := r.Header.Get("User-Agent")
	if len(ua) > maxUserAgentLen {
		ua = ua[:maxUserAgentLen]
	}
	return authuc.DeviceInfo{
		UserAgent:  ua,
		IPAddress:  realIP(r),
		DeviceName: useragent.Parse(ua),
		DeviceID:   deviceID,
	}
}
