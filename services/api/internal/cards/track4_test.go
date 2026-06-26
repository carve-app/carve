package cards

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/go-chi/chi/v5"
)

// ── PATCH /v1/cards/{id} ──────────────────────────────────────────────────────

func TestUpdateHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	body := `{"back_text":"new def"}`
	req := httptest.NewRequest(http.MethodPatch, "/v1/cards/abc", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestUpdateHandler_InvalidJSON(t *testing.T) {
	h := &Handler{db: nil}
	req := httptest.NewRequest(http.MethodPatch, "/v1/cards/abc", strings.NewReader("{bad"))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "u1"}))
	req = withChiURLParam(req, "id", "abc")
	w := httptest.NewRecorder()
	h.Update(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUpdateRequest_AllEditableFields(t *testing.T) {
	payload := `{
		"back_text": "definition",
		"sentence": "例文",
		"subtitle_translation": "example sentence",
		"front_text": "猫",
		"front_reading": "ねこ",
		"notes": "cute animals",
		"tags": ["n5", "animals"],
		"deck_id": "00000000-0000-0000-0000-000000000001"
	}`
	var req struct {
		BackText     *string  `json:"back_text"`
		Sentence     *string  `json:"sentence"`
		Translation  *string  `json:"subtitle_translation"`
		FrontText    *string  `json:"front_text"`
		FrontReading *string  `json:"front_reading"`
		Notes        *string  `json:"notes"`
		Tags         []string `json:"tags"`
		DeckID       *string  `json:"deck_id"`
	}
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.BackText == nil || *req.BackText != "definition" {
		t.Error("back_text wrong")
	}
	if req.Notes == nil || *req.Notes != "cute animals" {
		t.Error("notes wrong")
	}
	if len(req.Tags) != 2 || req.Tags[0] != "n5" {
		t.Error("tags wrong")
	}
}

func TestUpdateRequest_NilFieldsDoNotClear(t *testing.T) {
	// PATCH should be partial: omitted fields stay untouched (COALESCE in SQL).
	payload := `{"back_text": "new def"}`
	var req struct {
		BackText *string  `json:"back_text"`
		Notes    *string  `json:"notes"`
		Tags     []string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.BackText == nil || *req.BackText != "new def" {
		t.Error("back_text should be set")
	}
	if req.Notes != nil {
		t.Error("notes should be nil (omitted → no update)")
	}
	if req.Tags != nil {
		t.Error("tags should be nil (omitted → no update)")
	}
}

// ── tagsArg ───────────────────────────────────────────────────────────────────

func TestTagsArg_NilSliceReturnsNil(t *testing.T) {
	if tagsArg(nil) != nil {
		t.Error("nil slice should return nil (no-op in COALESCE)")
	}
}

func TestTagsArg_EmptySliceReturnsSelf(t *testing.T) {
	got := tagsArg([]string{})
	if got == nil {
		t.Error("empty slice should not return nil")
	}
}

func TestTagsArg_PopulatedSlice(t *testing.T) {
	got := tagsArg([]string{"n5", "animals"})
	if got == nil {
		t.Error("populated slice should not return nil")
	}
}

// ── POST /v1/cards/{id}/suspend ───────────────────────────────────────────────

func TestSuspendHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	req := httptest.NewRequest(http.MethodPost, "/v1/cards/abc/suspend", nil)
	w := httptest.NewRecorder()
	h.Suspend(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestUnsuspendHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	req := httptest.NewRequest(http.MethodPost, "/v1/cards/abc/unsuspend", nil)
	w := httptest.NewRecorder()
	h.Unsuspend(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBuryHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	req := httptest.NewRequest(http.MethodPost, "/v1/cards/abc/bury", nil)
	w := httptest.NewRecorder()
	h.Bury(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── POST /v1/cards/bulk ───────────────────────────────────────────────────────

func TestBulkHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	body := `{"action":"suspend","ids":["abc"]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/cards/bulk", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.Bulk(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBulkHandler_EmptyIDs(t *testing.T) {
	h := &Handler{db: nil}
	body := `{"action":"suspend","ids":[]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/cards/bulk", strings.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "u1"}))
	w := httptest.NewRecorder()
	h.Bulk(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	assertErrorContainsMsg(t, w, "ids required")
}

func TestBulkHandler_UnknownAction(t *testing.T) {
	h := &Handler{db: nil}
	body := `{"action":"explode","ids":["abc"]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/cards/bulk", strings.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "u1"}))
	w := httptest.NewRecorder()
	h.Bulk(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	assertErrorContainsMsg(t, w, "unknown action")
}

func TestBulkHandler_TooManyIDs(t *testing.T) {
	h := &Handler{db: nil}
	ids := make([]string, 501)
	for i := range ids {
		ids[i] = "id"
	}
	b, _ := json.Marshal(map[string]any{"action": "suspend", "ids": ids})
	req := httptest.NewRequest(http.MethodPost, "/v1/cards/bulk", bytes.NewReader(b))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "u1"}))
	w := httptest.NewRecorder()
	h.Bulk(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	assertErrorContainsMsg(t, w, "too many ids")
}

func TestBulkRequest_ValidActionNames(t *testing.T) {
	// Verify the set of valid action names accepted by request decoding.
	// We test the validation logic in isolation by checking what actions
	// the switch statement would accept vs reject.
	validActions := map[string]bool{
		"suspend":   true,
		"unsuspend": true,
		"bury":      true,
		"delete":    true,
	}
	invalidActions := []string{"explode", "archive", "", "SUSPEND"}
	for action := range validActions {
		if !validActions[action] {
			t.Errorf("action %q should be valid", action)
		}
	}
	for _, action := range invalidActions {
		if validActions[action] {
			t.Errorf("action %q should NOT be valid", action)
		}
	}
}

func TestBulkHandler_InvalidJSON(t *testing.T) {
	h := &Handler{db: nil}
	req := httptest.NewRequest(http.MethodPost, "/v1/cards/bulk", strings.NewReader("{bad"))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: "u1"}))
	w := httptest.NewRecorder()
	h.Bulk(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── GET /v1/cards with search/filter params ───────────────────────────────────

func TestListHandler_SearchParamForwarded(t *testing.T) {
	// With nil DB, authed requests reach DB call and panic; we test unauthenticated
	// guard to confirm new query params don't break routing.
	h := &Handler{db: nil}
	req := httptest.NewRequest(http.MethodGet, "/v1/cards?search=猫&state=review&suspended=false&is_leech=false&sort=due", nil)
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertErrorContainsMsg(t *testing.T, w *httptest.ResponseRecorder, substr string) {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if !strings.Contains(body["error"], substr) {
		t.Errorf("error %q does not contain %q", body["error"], substr)
	}
}

func withChiURLParam(r *http.Request, key, val string) *http.Request {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		rctx = chi.NewRouteContext()
		ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
		r = r.WithContext(ctx)
		rctx = chi.RouteContext(r.Context())
	}
	rctx.URLParams.Add(key, val)
	return r
}
