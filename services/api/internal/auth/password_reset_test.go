package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ── ForgotPassword ────────────────────────────────────────────────────────────

func TestForgotPassword_InvalidJSON(t *testing.T) {
	h := newAuthHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/forgot", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.ForgotPassword(w, req)
	// Always 204 — never leak information
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestForgotPassword_EmptyEmail(t *testing.T) {
	h := newAuthHandler()
	body, _ := json.Marshal(map[string]string{"email": ""})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/forgot", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ForgotPassword(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for empty email, got %d", w.Code)
	}
}

func TestForgotPassword_ValidEmailFormat_ReachesDB(t *testing.T) {
	// Validation passes → handler reaches DB call.
	// With nil pool this panics, so we only verify that invalid/empty cases
	// return 204 before touching the DB.
	// DB-dependent paths are covered by integration tests.
	t.Skip("requires real DB — tested in integration suite")
}

// ── ResetPassword ─────────────────────────────────────────────────────────────

func TestResetPassword_InvalidJSON(t *testing.T) {
	h := newAuthHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/reset", strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.ResetPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestResetPassword_MissingToken(t *testing.T) {
	h := newAuthHandler()
	body, _ := json.Marshal(map[string]string{"password": "newpassword123"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/reset", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ResetPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing token, got %d", w.Code)
	}
}

func TestResetPassword_ShortPassword(t *testing.T) {
	h := newAuthHandler()
	body, _ := json.Marshal(map[string]string{"token": "sometoken", "password": "short"})
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/reset", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ResetPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short password, got %d", w.Code)
	}
}

func TestResetPassword_InvalidToken_ReachesDB(t *testing.T) {
	// Validation passes → handler reaches DB call.
	// DB-dependent paths are covered by integration tests.
	t.Skip("requires real DB — tested in integration suite")
}
