package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAuthHandler() *Handler {
	return &Handler{db: nil}
}

// ── Register ──────────────────────────────────────────────────────────────────

func TestRegister_InvalidJSON(t *testing.T) {
	h := newAuthHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader("{bad json"))
	w := httptest.NewRecorder()
	h.Register(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRegister_MissingEmail(t *testing.T) {
	h := newAuthHandler()
	body, _ := json.Marshal(map[string]string{"password": "secret123", "display_name": "Test"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRegister_MissingPassword(t *testing.T) {
	h := newAuthHandler()
	body, _ := json.Marshal(map[string]string{"email": "a@b.com", "display_name": "Test"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRegister_MissingDisplayName(t *testing.T) {
	h := newAuthHandler()
	body, _ := json.Marshal(map[string]string{"email": "a@b.com", "password": "secret123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	h := newAuthHandler()
	body, _ := json.Marshal(map[string]string{
		"email": "a@b.com", "password": "short", "display_name": "Test",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short password, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "8 characters") {
		t.Errorf("expected '8 characters' in error, got: %s", w.Body.String())
	}
}

func TestRegister_SevenCharPasswordFails(t *testing.T) {
	// 7-char password is too short
	h := newAuthHandler()
	body, _ := json.Marshal(map[string]string{
		"email": "a@b.com", "password": "7chars!", "display_name": "Test",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Register(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("7-char password should fail validation, got %d", w.Code)
	}
}

// ── Login ─────────────────────────────────────────────────────────────────────

func TestLogin_InvalidJSON(t *testing.T) {
	h := newAuthHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLogin_MissingEmail(t *testing.T) {
	h := newAuthHandler()
	body, _ := json.Marshal(map[string]string{"password": "secret123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLogin_MissingPassword(t *testing.T) {
	h := newAuthHandler()
	body, _ := json.Marshal(map[string]string{"email": "a@b.com"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLogin_EmptyBody(t *testing.T) {
	h := newAuthHandler()
	body, _ := json.Marshal(map[string]string{"email": "", "password": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Login(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func TestRefresh_NoCookie(t *testing.T) {
	h := newAuthHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/refresh", nil)
	w := httptest.NewRecorder()
	h.Refresh(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with no cookie, got %d", w.Code)
	}
}

// ── Logout ────────────────────────────────────────────────────────────────────

func TestLogout_NoCookie_Returns204(t *testing.T) {
	// Logout with no cookie: skips DB call, clears cookie, returns 204
	h := newAuthHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	w := httptest.NewRecorder()
	h.Logout(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestLogout_ClearsRefreshCookie(t *testing.T) {
	h := newAuthHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	w := httptest.NewRecorder()
	h.Logout(w, req)
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshTokenCookie {
			if c.MaxAge >= 0 {
				t.Errorf("expected MaxAge < 0 to clear cookie, got %d", c.MaxAge)
			}
			return
		}
	}
	t.Errorf("expected refresh cookie to be set in response")
}

// ── Token helpers (unit tests for pure logic) ─────────────────────────────────

func TestHashRefreshToken_Deterministic(t *testing.T) {
	token := "test-refresh-token"
	h1 := HashRefreshToken(token)
	h2 := HashRefreshToken(token)
	if h1 != h2 {
		t.Errorf("HashRefreshToken is not deterministic")
	}
	if len(h1) == 0 {
		t.Errorf("HashRefreshToken returned empty string")
	}
	if h1 == token {
		t.Errorf("HashRefreshToken should transform the token, not return it as-is")
	}
}

func TestHashRefreshToken_DifferentInputsDifferentOutputs(t *testing.T) {
	h1 := HashRefreshToken("token-a")
	h2 := HashRefreshToken("token-b")
	if h1 == h2 {
		t.Errorf("different tokens should produce different hashes")
	}
}
