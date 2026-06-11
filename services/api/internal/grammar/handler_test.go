package grammar

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

// newAuthedRequest builds a request with valid auth claims injected into the
// context so handlers get past the auth check without middleware.
func newAuthedRequest(method, target string, body []byte) *http.Request {
	var b *bytes.Reader
	if body != nil {
		b = bytes.NewReader(body)
	} else {
		b = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, target, b)
	r.Header.Set("Content-Type", "application/json")
	claims := &auth.Claims{UserID: "test-user-id"}
	r = r.WithContext(auth.ContextWithClaims(context.Background(), claims))
	return r
}

func assertErrorContains(t *testing.T, w *httptest.ResponseRecorder, substr string) {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if !strings.Contains(body["error"], substr) {
		t.Errorf("error %q does not contain %q", body["error"], substr)
	}
}

// ── ListKnown ─────────────────────────────────────────────────────────────────

func TestListKnown_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodGet, "/v1/grammar/known?language=ja", nil)
	w := httptest.NewRecorder()
	h.ListKnown(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── MarkKnown ─────────────────────────────────────────────────────────────────

func TestMarkKnown_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodPost, "/v1/grammar/known", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	h.MarkKnown(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMarkKnown_InvalidJSON(t *testing.T) {
	h := &Handler{db: nil}
	r := newAuthedRequest(http.MethodPost, "/v1/grammar/known", []byte("not-json"))
	w := httptest.NewRecorder()
	h.MarkKnown(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	assertErrorContains(t, w, "invalid JSON")
}

func TestMarkKnown_MissingLanguageCode(t *testing.T) {
	h := &Handler{db: nil}
	body, _ := json.Marshal(map[string]string{"pattern_id": "te-iru"})
	r := newAuthedRequest(http.MethodPost, "/v1/grammar/known", body)
	w := httptest.NewRecorder()
	h.MarkKnown(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing language_code, got %d", w.Code)
	}
	assertErrorContains(t, w, "language_code and pattern_id are required")
}

func TestMarkKnown_MissingPatternID(t *testing.T) {
	h := &Handler{db: nil}
	body, _ := json.Marshal(map[string]string{"language_code": "ja"})
	r := newAuthedRequest(http.MethodPost, "/v1/grammar/known", body)
	w := httptest.NewRecorder()
	h.MarkKnown(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing pattern_id, got %d", w.Code)
	}
	assertErrorContains(t, w, "language_code and pattern_id are required")
}

func TestMarkKnown_BothMissing(t *testing.T) {
	h := &Handler{db: nil}
	body, _ := json.Marshal(map[string]string{})
	r := newAuthedRequest(http.MethodPost, "/v1/grammar/known", body)
	w := httptest.NewRecorder()
	h.MarkKnown(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── UnmarkKnown ───────────────────────────────────────────────────────────────

func TestUnmarkKnown_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodDelete, "/v1/grammar/known", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	h.UnmarkKnown(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestUnmarkKnown_InvalidJSON(t *testing.T) {
	h := &Handler{db: nil}
	r := newAuthedRequest(http.MethodDelete, "/v1/grammar/known", []byte("not-json"))
	w := httptest.NewRecorder()
	h.UnmarkKnown(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	assertErrorContains(t, w, "invalid JSON")
}

func TestUnmarkKnown_MissingFields(t *testing.T) {
	h := &Handler{db: nil}
	body, _ := json.Marshal(map[string]string{"language_code": "ja"})
	r := newAuthedRequest(http.MethodDelete, "/v1/grammar/known", body)
	w := httptest.NewRecorder()
	h.UnmarkKnown(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing pattern_id, got %d", w.Code)
	}
	assertErrorContains(t, w, "language_code and pattern_id are required")
}

// ── Response helpers ──────────────────────────────────────────────────────────

func TestWriteError_ResponseShape(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "test error")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if body["error"] != "test error" {
		t.Errorf("error field = %q, want %q", body["error"], "test error")
	}
}

func TestWriteJSON_ContentType(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"k": "v"})
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
