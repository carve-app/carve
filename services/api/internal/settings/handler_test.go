package settings

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

func TestPutFSRSRejectsUnsupportedLanguageBeforePersistence(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodPut, "/v1/settings/fsrs", strings.NewReader(`{"language_code":"AAA"}`))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: auth.NewID()}))
	w := httptest.NewRecorder()
	h.PutFSRS(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
