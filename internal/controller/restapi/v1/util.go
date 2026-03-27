package v1

import (
	"net"
	"net/http"
	"strings"
)

func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		for _, part := range strings.Split(xf, ",") {
			p := strings.TrimSpace(part)
			if p == "" {
				continue
			}
			if ip := net.ParseIP(p); ip != nil {
				return ip.String()
			}
			return p
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
