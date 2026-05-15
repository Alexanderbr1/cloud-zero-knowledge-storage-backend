package v1

import (
	"net/http"

	authuc "cloud-backend/internal/usecase/auth"
	"cloud-backend/pkg/useragent"
)

const maxUserAgentLen = 512

func parseDeviceInfo(r *http.Request) authuc.DeviceInfo {
	ua := r.Header.Get("User-Agent")
	if len(ua) > maxUserAgentLen {
		ua = ua[:maxUserAgentLen]
	}
	return authuc.DeviceInfo{
		UserAgent:  ua,
		IPAddress:  realIP(r),
		DeviceName: useragent.Parse(ua),
	}
}
