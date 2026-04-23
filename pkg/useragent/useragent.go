// Package useragent provides a minimal User-Agent string parser.
// No external dependencies — covers the 95 % case: Chrome / Firefox / Safari / Edge
// on Windows / macOS / Linux / iOS / Android.
package useragent

import (
	"strings"
)

// Parse returns a human-readable device label derived from the User-Agent header,
// e.g. "Chrome on macOS" or "Safari on iPhone".
func Parse(ua string) string {
	if ua == "" {
		return "Unknown device"
	}
	os := parseOS(ua)
	browser := parseBrowser(ua)
	switch {
	case browser != "" && os != "":
		return browser + " on " + os
	case browser != "":
		return browser
	case os != "":
		return "Browser on " + os
	default:
		return "Unknown device"
	}
}

func parseBrowser(ua string) string {
	switch {
	case has(ua, "Edg/") || has(ua, "Edge/"):
		return "Edge"
	case has(ua, "OPR/") || has(ua, "Opera/"):
		return "Opera"
	// Chrome must come before Safari (Chrome UA also contains "Safari").
	case has(ua, "Chrome/") && !has(ua, "Chromium/"):
		return "Chrome"
	case has(ua, "Chromium/"):
		return "Chromium"
	case has(ua, "Firefox/"):
		return "Firefox"
	case has(ua, "Safari/") && has(ua, "Version/"):
		return "Safari"
	default:
		return ""
	}
}

func parseOS(ua string) string {
	switch {
	case has(ua, "iPhone"):
		return "iPhone"
	case has(ua, "iPad"):
		return "iPad"
	case has(ua, "Android"):
		return "Android"
	case has(ua, "Windows NT"):
		return "Windows"
	case has(ua, "Macintosh") || has(ua, "Mac OS X"):
		return "macOS"
	case has(ua, "Linux"):
		return "Linux"
	default:
		return ""
	}
}

func has(s, sub string) bool { return strings.Contains(s, sub) }
