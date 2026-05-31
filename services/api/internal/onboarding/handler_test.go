package onboarding

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

func newOnboardingHandler() *Handler {
	return &Handler{db: nil}
}

func authedCtx(r *http.Request) *http.Request {
	return r.WithContext(auth.ContextWithClaims(r.Context(), &auth.Claims{UserID: "user-test-001"}))
}

// ── Auth guards ───────────────────────────────────────────────────────────────

func TestKnownWords_NoAuth(t *testing.T) {
	h := newOnboardingHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/known-words", nil)
	w := httptest.NewRecorder()
	h.KnownWords(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestStarterDeck_NoAuth(t *testing.T) {
	h := newOnboardingHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/starter-deck", nil)
	w := httptest.NewRecorder()
	h.StarterDeck(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── KnownWords validation ──────────────────────────────────────────────────────

func TestKnownWords_InvalidJSON(t *testing.T) {
	h := newOnboardingHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/known-words", strings.NewReader("{bad json"))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.KnownWords(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestKnownWords_MissingLanguage(t *testing.T) {
	h := newOnboardingHandler()
	body, _ := json.Marshal(map[string]any{"lemmas": []string{"食べる", "飲む"}})
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/known-words", bytes.NewReader(body))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.KnownWords(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing language, got %d", w.Code)
	}
}

func TestKnownWords_EmptyLemmas(t *testing.T) {
	h := newOnboardingHandler()
	body, _ := json.Marshal(map[string]any{"language": "ja", "lemmas": []string{}})
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/known-words", bytes.NewReader(body))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.KnownWords(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty lemmas, got %d", w.Code)
	}
}

func TestKnownWords_NullLemmas(t *testing.T) {
	h := newOnboardingHandler()
	body, _ := json.Marshal(map[string]any{"language": "ja"})
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/known-words", bytes.NewReader(body))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.KnownWords(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for null lemmas, got %d", w.Code)
	}
}

func TestKnownWords_MissingBothFields(t *testing.T) {
	h := newOnboardingHandler()
	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/known-words", bytes.NewReader(body))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.KnownWords(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty body, got %d", w.Code)
	}
}

// ── StarterDeck validation ────────────────────────────────────────────────────

func TestStarterDeck_InvalidJSON(t *testing.T) {
	h := newOnboardingHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/starter-deck", strings.NewReader("{bad"))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.StarterDeck(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestStarterDeck_MissingLanguage(t *testing.T) {
	h := newOnboardingHandler()
	body, _ := json.Marshal(map[string]any{})
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/starter-deck", bytes.NewReader(body))
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.StarterDeck(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing language, got %d", w.Code)
	}
}
