package useragent

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want string
	}{
		// empty / unknown
		{"empty", "", "Unknown device"},
		{"garbage", "curl/7.88.1", "Unknown device"},
		{"only OS token", "Mozilla/5.0 (Windows NT 10.0)", "Browser on Windows"},
		{"only browser token", "Firefox/120.0", "Firefox"},

		// Chrome — must not be misidentified as Safari (Chrome UA includes "Safari")
		{"Chrome macOS", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36", "Chrome on macOS"},
		{"Chrome Windows", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36", "Chrome on Windows"},
		{"Chrome Linux", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36", "Chrome on Linux"},
		{"Chrome Android", "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.6367.82 Mobile Safari/537.36", "Chrome on Android"},

		// Safari — only real Safari has "Version/" alongside "Safari/"
		{"Safari macOS", "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15", "Safari on macOS"},
		{"Safari iPhone", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1", "Safari on iPhone"},
		{"Safari iPad", "Mozilla/5.0 (iPad; CPU OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1", "Safari on iPad"},

		// Edge — takes priority over Chrome
		{"Edge Windows", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0", "Edge on Windows"},

		// Firefox
		{"Firefox Windows", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0", "Firefox on Windows"},
		{"Firefox Linux", "Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0", "Firefox on Linux"},
		{"Firefox Android", "Mozilla/5.0 (Android 14; Mobile; rv:125.0) Gecko/125.0 Firefox/125.0", "Firefox on Android"},

		// Opera
		{"Opera Windows", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 OPR/110.0.0.0", "Opera on Windows"},

		// Chromium
		{"Chromium Linux", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chromium/124.0.0.0 Chrome/124.0.0.0 Safari/537.36", "Chromium on Linux"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.ua)
			if got != tc.want {
				t.Errorf("Parse(%q)\n  want %q\n  got  %q", tc.ua, tc.want, got)
			}
		})
	}
}
