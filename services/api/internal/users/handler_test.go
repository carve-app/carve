package users

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

func newUsersHandler() *Handler {
	return &Handler{db: nil}
}

func authedCtx(r *http.Request) *http.Request {
	return r.WithContext(auth.ContextWithClaims(r.Context(), &auth.Claims{UserID: "user-test-001"}))
}

// ── Auth guards ───────────────────────────────────────────────────────────────

func TestMe_NoAuth(t *testing.T) {
	h := newUsersHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	w := httptest.NewRecorder()
	h.Me(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDelete_NoAuth(t *testing.T) {
	h := newUsersHandler()
	req := httptest.NewRequest(http.MethodDelete, "/v1/users/me", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestUpdate_NoAuth(t *testing.T) {
	h := newUsersHandler()
	req := httptest.NewRequest(http.MethodPatch, "/v1/users/me", nil)
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── Update validation ─────────────────────────────────────────────────────────

func TestUpdate_InvalidJSON(t *testing.T) {
	h := newUsersHandler()
	req := httptest.NewRequest(http.MethodPatch, "/v1/users/me", strings.NewReader("{bad json"))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestUpdate_EmptyBody: empty JSON `{}` is valid (no fields to change),
// but the handler then calls Me which queries DB → only test auth guard here.

// ── Response headers ──────────────────────────────────────────────────────────

func TestMe_ResponseIsJSON(t *testing.T) {
	// Auth guard returns JSON error
	h := newUsersHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	w := httptest.NewRecorder()
	h.Me(w, req)
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") && w.Body.String() != "" {
		// The 401 here uses http.Error which is text/plain, but we check the body is valid JSON
		var m map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Errorf("expected JSON body, got: %s", w.Body.String())
		}
	}
}
