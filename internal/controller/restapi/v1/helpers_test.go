package v1

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

// b64 encodes bytes to standard base64 (same encoding used by mustDecodeB64).
func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func rec() *httptest.ResponseRecorder { return httptest.NewRecorder() }

func TestMustDecodeB64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		minLen  int
		maxLen  int
		wantOK  bool
		wantLen int // only checked when wantOK == true
	}{
		// ── invalid base64 ─────────────────────────────────────────────────────
		{"invalid base64 chars", "not!base64", 0, 0, false, 0},
		{"wrong padding", "YQ", 0, 0, false, 0}, // "a" needs "YQ=="
		{"empty string", "", 0, 0, true, 0},     // empty is valid base64 → zero bytes

		// ── minLen enforcement ─────────────────────────────────────────────────
		{"below minLen", b64(make([]byte, 3)), 4, 0, false, 0},
		{"exactly at minLen", b64(make([]byte, 4)), 4, 0, true, 4},
		{"above minLen", b64(make([]byte, 10)), 4, 0, true, 10},

		// ── maxLen enforcement ─────────────────────────────────────────────────
		{"above maxLen", b64(make([]byte, 5)), 0, 4, false, 0},
		{"exactly at maxLen", b64(make([]byte, 4)), 0, 4, true, 4},
		{"below maxLen", b64(make([]byte, 3)), 0, 4, true, 3},

		// ── maxLen = 0 means no upper bound ────────────────────────────────────
		{"large value, no upper bound", b64(make([]byte, 1000)), 1, 0, true, 1000},

		// ── exact bounds (minLen == maxLen, fixed-width field) ─────────────────
		{"exact size match", b64(make([]byte, 40)), 40, 40, true, 40},
		{"one byte short for fixed field", b64(make([]byte, 39)), 40, 40, false, 0},
		{"one byte over for fixed field", b64(make([]byte, 41)), 40, 40, false, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := rec()
			got, ok := mustDecodeB64(w, tc.input, "test_field", tc.minLen, tc.maxLen)

			if ok != tc.wantOK {
				t.Errorf("ok: want %v, got %v", tc.wantOK, ok)
			}
			if !tc.wantOK {
				if w.Code != http.StatusBadRequest {
					t.Errorf("on failure: want 400, got %d", w.Code)
				}
				return
			}
			if len(got) != tc.wantLen {
				t.Errorf("len: want %d, got %d", tc.wantLen, len(got))
			}
		})
	}
}
