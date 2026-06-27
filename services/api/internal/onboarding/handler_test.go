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

func TestPlacementTest_NoAuth(t *testing.T) {
	h := newOnboardingHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/onboarding/placement-test?language=en", nil)
	w := httptest.NewRecorder()
	h.PlacementTest(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSubmitPlacementTest_NoAuth(t *testing.T) {
	h := newOnboardingHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/placement-test", nil)
	w := httptest.NewRecorder()
	h.SubmitPlacementTest(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestPlacementTest_ReturnsEnglishItems(t *testing.T) {
	h := newOnboardingHandler()
	req := authedCtx(httptest.NewRequest(http.MethodGet, "/v1/onboarding/placement-test?language=en", nil))
	w := httptest.NewRecorder()
	h.PlacementTest(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var payload placementTestPayload
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Language != "en" || payload.Version != englishPlacementVersion || len(payload.Items) != 30 {
		t.Fatalf("unexpected placement payload: language=%q version=%q items=%d", payload.Language, payload.Version, len(payload.Items))
	}
}

func TestPlacementTest_RejectsUnsupportedLanguage(t *testing.T) {
	h := newOnboardingHandler()
	req := authedCtx(httptest.NewRequest(http.MethodGet, "/v1/onboarding/placement-test?language=ja", nil))
	w := httptest.NewRecorder()
	h.PlacementTest(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSubmitPlacementTest_ValidatesBeforeDatabase(t *testing.T) {
	h := newOnboardingHandler()
	for name, body := range map[string]string{
		"invalid json":      "{bad",
		"unsupported":       `{"language":"ja","version":"en-receptive-v1","answers":[]}`,
		"missing version":   `{"language":"en","answers":[]}`,
		"missing questions": `{"language":"en","version":"en-receptive-v1","answers":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := authedCtx(httptest.NewRequest(http.MethodPost, "/v1/onboarding/placement-test", strings.NewReader(body)))
			w := httptest.NewRecorder()
			h.SubmitPlacementTest(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
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
