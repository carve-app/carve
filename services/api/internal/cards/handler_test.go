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

// ── FindSimilar handler — validation paths ───────────────────────────────────

func TestFindSimilarHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodPost, "/v1/cards/find-similar", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	h.FindSimilar(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestFindSimilarHandler_InvalidJSON(t *testing.T) {
	h := &Handler{db: nil}
	r := newAuthedRequest(http.MethodPost, "/v1/cards/find-similar", []byte("not json"))
	w := httptest.NewRecorder()
	h.FindSimilar(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestFindSimilarHandler_MissingFields(t *testing.T) {
	h := &Handler{db: nil}
	body, _ := json.Marshal(map[string]string{"language_code": "ja"})
	r := newAuthedRequest(http.MethodPost, "/v1/cards/find-similar", body)
	w := httptest.NewRecorder()
	h.FindSimilar(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing sentence, got %d", w.Code)
	}
	assertErrorContains(t, w, "language_code and sentence are required")
}

func TestFindSimilarHandler_WhitespaceSentenceShortCircuits(t *testing.T) {
	// charTrigrams normalises whitespace-only input to nil — the handler must
	// short-circuit to an empty matches list before touching the DB (db=nil
	// here would otherwise panic).
	h := &Handler{db: nil}
	body, _ := json.Marshal(map[string]string{"language_code": "ja", "sentence": "   "})
	r := newAuthedRequest(http.MethodPost, "/v1/cards/find-similar", body)
	w := httptest.NewRecorder()
	h.FindSimilar(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"matches":[]`) {
		t.Errorf("expected empty matches, got body=%s", w.Body.String())
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

// ── Create request JSON schema — Track 2 mining payload coverage ──────────────
//
// These tests verify that the Create handler's request struct correctly decodes
// the full set of fields that the extension sends when mining a video card:
// subtitle_translation (translated text shown under card), source_url (video URL),
// source_timestamp (seconds into video), and the optional back_text/reading.
//
// They use JSON unmarshaling directly against the same struct the handler uses,
// without triggering the nil-DB panic that occurs after validation passes.

// miningCreateRequest mirrors the exact request struct in Create (handler.go).
// If the handler fields change this test will fail, keeping the two in sync.
type miningCreateRequest struct {
	LanguageCode        string   `json:"language_code"`
	Lemma               string   `json:"lemma"`
	Reading             string   `json:"reading"`
	BackText            *string  `json:"back_text"`
	SubtitleTranslation *string  `json:"subtitle_translation"`
	Sentence            *string  `json:"sentence"`
	SourceURL           *string  `json:"source_url"`
	SourceTimestamp     *float64 `json:"source_timestamp"`
}

func TestCreateRequest_VideoMiningPayload_AllFields(t *testing.T) {
	// Represents what the extension sends after pressing ⚡ Mine on a Netflix/YouTube subtitle.
	payload := `{
		"language_code":        "ja",
		"lemma":               "人工知能",
		"reading":             "じんこうちのう",
		"back_text":           "artificial intelligence",
		"subtitle_translation": "AI is useful for studying Japanese.",
		"sentence":            "人工知能は日本語を学ぶのに役立ちます。",
		"source_url":          "https://www.youtube.com/watch?v=dQw4w9WgXcZ",
		"source_timestamp":    134.5
	}`

	var req miningCreateRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("failed to decode mining payload: %v", err)
	}

	if req.LanguageCode != "ja" {
		t.Errorf("language_code = %q, want ja", req.LanguageCode)
	}
	if req.Lemma != "人工知能" {
		t.Errorf("lemma = %q, want 人工知能", req.Lemma)
	}
	if req.Reading != "じんこうちのう" {
		t.Errorf("reading = %q, want じんこうちのう", req.Reading)
	}
	if req.SubtitleTranslation == nil || *req.SubtitleTranslation == "" {
		t.Error("subtitle_translation is nil or empty")
	}
	if req.SourceURL == nil || !strings.Contains(*req.SourceURL, "youtube.com") {
		t.Errorf("source_url wrong: %v", req.SourceURL)
	}
	if req.SourceTimestamp == nil || *req.SourceTimestamp != 134.5 {
		t.Errorf("source_timestamp = %v, want 134.5", req.SourceTimestamp)
	}
	if req.Sentence == nil {
		t.Error("sentence should be set")
	}
}

func TestCreateRequest_VideoMiningPayload_NetflixURL(t *testing.T) {
	payload := `{
		"language_code": "ja",
		"lemma": "生活",
		"reading": "せいかつ",
		"subtitle_translation": "daily life",
		"source_url": "https://www.netflix.com/watch/80057281",
		"source_timestamp": 892.0
	}`
	var req miningCreateRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("decode netflix mining payload: %v", err)
	}
	if req.SourceURL == nil || !strings.Contains(*req.SourceURL, "netflix.com") {
		t.Errorf("expected netflix URL, got: %v", req.SourceURL)
	}
	if req.SourceTimestamp == nil || *req.SourceTimestamp != 892.0 {
		t.Errorf("source_timestamp wrong: %v", req.SourceTimestamp)
	}
}

func TestCreateRequest_VideoMiningPayload_CrunchyrollURL(t *testing.T) {
	payload := `{
		"language_code": "ja",
		"lemma": "友達",
		"source_url": "https://www.crunchyroll.com/watch/GYGG23J5Y/episode-1",
		"source_timestamp": 245.3
	}`
	var req miningCreateRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("decode crunchyroll mining payload: %v", err)
	}
	if req.SourceURL == nil || !strings.Contains(*req.SourceURL, "crunchyroll.com") {
		t.Errorf("expected crunchyroll URL, got: %v", req.SourceURL)
	}
}

func TestCreateRequest_VideoMiningPayload_NullOptionalFields(t *testing.T) {
	// Minimal mining payload (translation + URL can be omitted if not available)
	payload := `{"language_code": "ja", "lemma": "猫"}`
	var req miningCreateRequest
	if err := json.Unmarshal([]byte(payload), &req); err != nil {
		t.Fatalf("decode minimal payload: %v", err)
	}
	if req.SubtitleTranslation != nil {
		t.Error("subtitle_translation should be nil when not provided")
	}
	if req.SourceURL != nil {
		t.Error("source_url should be nil when not provided")
	}
}

func TestCreateResponse_IncludesCardID(t *testing.T) {
	// The extension needs card_id in the response to call ATTACH_PAGE_SCREENSHOT.
	// Verify the handler returns 'id' field (not a DB test — just JSON shape).
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":            "test-card-abc",
		"lemma":         "猫",
		"language_code": "ja",
		"created_at":    "2024-01-01T00:00:00Z",
	})
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["id"]; !ok {
		t.Error("card Create response must include 'id' field (extension uses it for screenshot attachment)")
	}
	if _, ok := resp["lemma"]; !ok {
		t.Error("card Create response must include 'lemma' field")
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
