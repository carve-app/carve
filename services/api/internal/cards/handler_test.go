package cards

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/jackc/pgx/v5/pgconn"
)

// ── formInt ──────────────────────────────────────────────────────────────────

func TestFormInt(t *testing.T) {
	cases := []struct {
		field string
		form  map[string]string
		want  *int
	}{
		{"n", map[string]string{"n": "42"}, intPtr(42)},
		{"n", map[string]string{"n": "0"}, intPtr(0)},
		{"n", map[string]string{"n": "-5"}, intPtr(-5)},
		{"n", map[string]string{"n": ""}, nil},
		{"n", map[string]string{}, nil},
		{"n", map[string]string{"n": "abc"}, nil},
		{"n", map[string]string{"n": "3.14"}, nil},
		{"n", map[string]string{"n": " 7 "}, nil}, // strconv.Atoi rejects whitespace
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.Form = make(map[string][]string)
		for k, v := range tc.form {
			r.Form[k] = []string{v}
		}
		got := formInt(r, tc.field)
		if tc.want == nil && got != nil {
			t.Errorf("formInt(%q) = %v, want nil", tc.form, *got)
		} else if tc.want != nil && got == nil {
			t.Errorf("formInt(%q) = nil, want %d", tc.form, *tc.want)
		} else if tc.want != nil && *got != *tc.want {
			t.Errorf("formInt(%q) = %d, want %d", tc.form, *got, *tc.want)
		}
	}
}

func intPtr(n int) *int { return &n }

// ── isUniqueViolation ─────────────────────────────────────────────────────────

func TestIsUniqueViolation(t *testing.T) {
	pgUnique := &pgconn.PgError{Code: "23505"}
	pgOther := &pgconn.PgError{Code: "23503"}

	if !isUniqueViolation(pgUnique) {
		t.Error("expected true for 23505")
	}
	if isUniqueViolation(pgOther) {
		t.Error("expected false for 23503 (foreign key)")
	}
	if isUniqueViolation(fmt.Errorf("not a pg error")) {
		t.Error("expected false for non-pg error")
	}
	if isUniqueViolation(nil) {
		t.Error("expected false for nil")
	}
}

// ── Create handler — validation paths (no DB) ─────────────────────────────────

// newAuthedRequest builds a request with valid auth claims injected into the
// context. The handler is called directly (no middleware needed).
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

func TestCreateHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodPost, "/v1/cards", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestCreateHandler_InvalidJSON(t *testing.T) {
	h := &Handler{db: nil}
	r := newAuthedRequest(http.MethodPost, "/v1/cards", []byte("not-json"))
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	assertErrorContains(t, w, "invalid JSON")
}

func TestCreateHandler_MissingLanguageCode(t *testing.T) {
	h := &Handler{db: nil}
	body, _ := json.Marshal(map[string]string{"lemma": "食べる"})
	r := newAuthedRequest(http.MethodPost, "/v1/cards", body)
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing language_code, got %d", w.Code)
	}
}

func TestCreateHandler_MissingLemma(t *testing.T) {
	h := &Handler{db: nil}
	body, _ := json.Marshal(map[string]string{"language_code": "ja"})
	r := newAuthedRequest(http.MethodPost, "/v1/cards", body)
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing lemma, got %d", w.Code)
	}
	assertErrorContains(t, w, "language_code and lemma are required")
}

func TestCreateHandler_BothMissing(t *testing.T) {
	h := &Handler{db: nil}
	body, _ := json.Marshal(map[string]string{})
	r := newAuthedRequest(http.MethodPost, "/v1/cards", body)
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── Get handler — validation paths ───────────────────────────────────────────

func TestGetHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodGet, "/v1/cards/some-id", nil)
	w := httptest.NewRecorder()
	h.Get(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── Delete handler — validation paths ────────────────────────────────────────

func TestDeleteHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodDelete, "/v1/cards/some-id", nil)
	w := httptest.NewRecorder()
	h.Delete(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── AttachMedia handler — validation paths ────────────────────────────────────

func TestAttachMediaHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodPost, "/v1/cards/some-id/media", nil)
	w := httptest.NewRecorder()
	h.AttachMedia(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── List handler — validation paths ──────────────────────────────────────────

func TestListHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodGet, "/v1/cards", nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── Response structure ────────────────────────────────────────────────────────

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
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

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
