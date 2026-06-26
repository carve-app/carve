package immersion

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

func TestCreateRejectsUnsupportedLanguageBeforePersistence(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/immersion", strings.NewReader(`{
		"language_code":"AAA",
		"session_type":"reading",
		"duration_sec":60,
		"started_at":"2026-06-26T12:00:00Z"
	}`))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: auth.NewID()}))
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
