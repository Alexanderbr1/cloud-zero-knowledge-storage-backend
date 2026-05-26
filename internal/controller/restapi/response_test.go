package restapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cloud-backend/internal/controller/restapi"
)

// ─── DecodeJSON ───────────────────────────────────────────────────────────────

type testPayload struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestDecodeJSON_HappyPath(t *testing.T) {
	body := `{"name":"alice","value":42}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var got testPayload
	if err := restapi.DecodeJSON(req, &got); err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if got.Name != "alice" || got.Value != 42 {
		t.Errorf("decoded: want {alice 42}, got %+v", got)
	}
}

func TestDecodeJSON_NilBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = nil

	var got testPayload
	if err := restapi.DecodeJSON(req, &got); err == nil {
		t.Fatal("want error for nil body")
	}
}

func TestDecodeJSON_InvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{not valid json}"))

	var got testPayload
	if err := restapi.DecodeJSON(req, &got); err == nil {
		t.Fatal("want error for invalid JSON")
	}
}

func TestDecodeJSON_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))

	var got testPayload
	if err := restapi.DecodeJSON(req, &got); err == nil {
		t.Fatal("want error for empty body (EOF)")
	}
}

func TestDecodeJSON_SizeLimit_Truncates(t *testing.T) {
	// Build a JSON body that exceeds the 4 MiB limit. The value string fills most
	// of the budget; the closing `"` and `}` land beyond the limit so the decoder
	// sees truncated input and returns an error.
	const limit = 4 << 20
	var buf bytes.Buffer
	buf.WriteString(`{"name":"`)
	buf.Write(bytes.Repeat([]byte("x"), limit)) // pushes closing chars past the limit
	buf.WriteString(`"}`)

	req := httptest.NewRequest(http.MethodPost, "/", &buf)

	var got testPayload
	if err := restapi.DecodeJSON(req, &got); err == nil {
		t.Fatal("want error when body exceeds 4 MiB limit")
	}
}

// ─── WriteJSON ────────────────────────────────────────────────────────────────

func TestWriteJSON_SetsContentTypeAndStatus(t *testing.T) {
	w := httptest.NewRecorder()
	restapi.WriteJSON(w, http.StatusCreated, map[string]string{"key": "val"})

	if w.Code != http.StatusCreated {
		t.Errorf("status: want 201, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}
}

func TestWriteJSON_BodyIsValidJSON(t *testing.T) {
	type payload struct {
		ID int `json:"id"`
	}
	w := httptest.NewRecorder()
	restapi.WriteJSON(w, http.StatusOK, payload{ID: 7})

	var got payload
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if got.ID != 7 {
		t.Errorf("ID: want 7, got %d", got.ID)
	}
}

// ─── WriteError ───────────────────────────────────────────────────────────────

func TestWriteError_ContainsErrorField(t *testing.T) {
	w := httptest.NewRecorder()
	restapi.WriteError(w, http.StatusUnauthorized, "unauthorized")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status: want 401, got %d", w.Code)
	}
	var got map[string]string
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if got["error"] != "unauthorized" {
		t.Errorf(`error field: want "unauthorized", got %q`, got["error"])
	}
}

// ─── WriteInternalError ───────────────────────────────────────────────────────

func TestWriteInternalError_Returns500(t *testing.T) {
	w := httptest.NewRecorder()
	restapi.WriteInternalError(w, nopLog, fmt.Errorf("something broke"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", w.Code)
	}
	var got map[string]string
	json.NewDecoder(w.Body).Decode(&got) //nolint
	if got["error"] != "internal error" {
		t.Errorf(`error field: want "internal error", got %q`, got["error"])
	}
}
