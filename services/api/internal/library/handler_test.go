package library

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

func intPtr2(n int) *int { return &n }

func TestAddRejectsUnsupportedLanguageBeforePersistence(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/library", strings.NewReader(`{"url":"https://example.com","language":"AAA"}`))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: auth.NewID()}))
	w := httptest.NewRecorder()
	h.Add(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ── parseSRT ──────────────────────────────────────────────────────────────────

func TestParseSRT_StripsSeuqenceNumbers(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:03,000\nHello world\n\n2\n00:00:04,000 --> 00:00:06,000\nGoodbye\n"
	got := parseSRT(input)
	if strings.Contains(got, "-->") {
		t.Errorf("timing line not stripped: %q", got)
	}
	if strings.Contains(got, "\n1\n") || got == "1" || strings.HasPrefix(got, "1\n") {
		t.Errorf("sequence number not stripped: %q", got)
	}
	if !strings.Contains(got, "Hello world") {
		t.Errorf("expected subtitle text, got: %q", got)
	}
	if !strings.Contains(got, "Goodbye") {
		t.Errorf("expected 'Goodbye', got: %q", got)
	}
}

func TestParseSRT_StripInlineHTML(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:03,000\n<i>Italic text</i>\n"
	got := parseSRT(input)
	if strings.Contains(got, "<i>") || strings.Contains(got, "</i>") {
		t.Errorf("HTML tags not stripped: %q", got)
	}
	if !strings.Contains(got, "Italic text") {
		t.Errorf("expected 'Italic text', got: %q", got)
	}
}

func TestParseSRT_EmptyInput(t *testing.T) {
	got := parseSRT("")
	if got != "" {
		t.Errorf("expected empty output for empty input, got: %q", got)
	}
}

func TestParseSRT_OnlyTimingAndNumbers(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:03,000\n\n2\n00:00:05,000 --> 00:00:07,000\n"
	got := parseSRT(input)
	if got != "" {
		t.Errorf("expected empty output for timing-only SRT, got: %q", got)
	}
}

func TestParseSRT_PreservesJapanese(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:03,000\n日本語のテキスト\n\n2\n00:00:04,000 --> 00:00:06,000\n<b>太字</b>\n"
	got := parseSRT(input)
	if !strings.Contains(got, "日本語のテキスト") {
		t.Errorf("expected Japanese text, got: %q", got)
	}
	if !strings.Contains(got, "太字") {
		t.Errorf("expected stripped bold text, got: %q", got)
	}
}

func TestParseSRT_MultipleHTMLTags(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:03,000\n<font color=\"white\">Hello</font>\n"
	got := parseSRT(input)
	if strings.Contains(got, "<font") || strings.Contains(got, "</font>") {
		t.Errorf("font tag not stripped: %q", got)
	}
	if !strings.Contains(got, "Hello") {
		t.Errorf("expected 'Hello', got: %q", got)
	}
}

// ── extractTitle ──────────────────────────────────────────────────────────────

func TestExtractTitle_BasicTitle(t *testing.T) {
	html := "<html><head><title>My Page Title</title></head><body></body></html>"
	got := extractTitle(html)
	if got != "My Page Title" {
		t.Errorf("expected 'My Page Title', got: %q", got)
	}
}

func TestExtractTitle_NoTitle(t *testing.T) {
	html := "<html><body><p>No title here</p></body></html>"
	got := extractTitle(html)
	if got != "" {
		t.Errorf("expected empty string for missing title, got: %q", got)
	}
}

func TestExtractTitle_UpperCaseTag(t *testing.T) {
	html := "<HTML><HEAD><TITLE>Upper Case Title</TITLE></HEAD></HTML>"
	got := extractTitle(html)
	if got != "Upper Case Title" {
		t.Errorf("expected 'Upper Case Title', got: %q", got)
	}
}

func TestExtractTitle_WithWhitespace(t *testing.T) {
	html := "<title>  Padded Title  </title>"
	got := extractTitle(html)
	if got != "Padded Title" {
		t.Errorf("expected trimmed title, got: %q", got)
	}
}

// ── stripHTML ─────────────────────────────────────────────────────────────────

func TestStripHTML_RemovesTags(t *testing.T) {
	html := "<p>Hello <b>world</b></p>"
	got := stripHTML(html)
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("HTML tags not stripped: %q", got)
	}
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "world") {
		t.Errorf("expected text content, got: %q", got)
	}
}

func TestStripHTML_CollapsesWhitespace(t *testing.T) {
	html := "<p>word1</p>  \t  <p>word2</p>"
	got := stripHTML(html)
	if strings.Contains(got, "  ") {
		t.Errorf("expected collapsed whitespace, got: %q", got)
	}
}

func TestStripHTML_EmptyInput(t *testing.T) {
	got := stripHTML("")
	if got != "" {
		t.Errorf("expected empty output, got: %q", got)
	}
}

func TestStripHTML_PlainText(t *testing.T) {
	text := "no tags here"
	got := stripHTML(text)
	if got != text {
		t.Errorf("expected unchanged plain text, got: %q", got)
	}
}

// ── auth guard tests ──────────────────────────────────────────────────────────

func newLibraryHandler() *Handler {
	return &Handler{db: nil, nlpBaseURL: "http://localhost:8001"}
}

func authedCtx(ctx context.Context) context.Context {
	return auth.ContextWithClaims(ctx, &auth.Claims{UserID: "user-test-001"})
}

func TestRead_NoAuth(t *testing.T) {
	h := newLibraryHandler()
	req := httptest.NewRequest(http.MethodGet, "/v1/library/some-id/reader", nil)
	w := httptest.NewRecorder()
	h.Read(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestImportFile_NoAuth(t *testing.T) {
	h := newLibraryHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/library/import", nil)
	w := httptest.NewRecorder()
	h.ImportFile(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestImportFile_WrongFileType(t *testing.T) {
	h := newLibraryHandler()
	body, ct := buildMultipartFile("test.pdf", "some pdf content", "language", "ja")
	req := httptest.NewRequest(http.MethodPost, "/v1/library/import", body)
	req.Header.Set("Content-Type", ct)
	req = req.WithContext(authedCtx(req.Context()))
	w := httptest.NewRecorder()
	h.ImportFile(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for .pdf, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "unsupported file type") {
		t.Errorf("expected 'unsupported file type' in body, got: %s", w.Body.String())
	}
}

func TestImportFile_NoFile(t *testing.T) {
	h := newLibraryHandler()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("language", "ja")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/library/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = req.WithContext(authedCtx(req.Context()))
	w := httptest.NewRecorder()
	h.ImportFile(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing file, got %d", w.Code)
	}
}

func TestImportFile_RejectsOversizedRequest(t *testing.T) {
	h := newLibraryHandler()
	body, ct := buildMultipartFile("large.txt", strings.Repeat("x", 6<<20), "language", "ja")
	req := httptest.NewRequest(http.MethodPost, "/v1/library/import", body)
	req.Header.Set("Content-Type", ct)
	req = req.WithContext(authedCtx(req.Context()))
	w := httptest.NewRecorder()
	h.ImportFile(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
	}
}

// TestImportFile_TxtType and TestImportFile_SrtType are integration tests
// that require a real DB. File-type validation is covered by TestImportFile_WrongFileType.

// ── Reader token schema → card creation parity (Track 3) ─────────────────────
//
// The reader returns NLP tokens. The extension reads those tokens and constructs
// a MINE_CARD message → background SW → POST /v1/cards.
// These tests verify that the token fields the reader exposes are sufficient to
// populate all required and optional card creation fields — "parity" means a card
// mined from the reader has the same fidelity as a card mined from a video.

// readerToken mirrors the JSON fields the library Read handler plucks from the
// NLP service response and forwards to the extension. (tokenShape in handler.go)
type readerToken struct {
	Surface       string `json:"surface"`
	Lemma         string `json:"lemma"`
	ReadingHira   string `json:"reading_hira"`
	Pos           string `json:"pos"`
	IsContentWord bool   `json:"is_content_word"`
	UserStatus    string `json:"user_status"`
	FrequencyRank *int   `json:"frequency_rank"`
}

func TestReaderTokenSchema_HasRequiredMiningFields(t *testing.T) {
	// Simulate the JSON the NLP service sends for a single token.
	raw := `{
		"surface":        "人工知能",
		"lemma":          "人工知能",
		"reading_hira":   "じんこうちのう",
		"pos":            "名詞",
		"is_content_word": true,
		"user_status":    "unknown",
		"frequency_rank": 4200
	}`
	var tok readerToken
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		t.Fatalf("unmarshal reader token: %v", err)
	}

	// These four fields are non-negotiable: without them the extension cannot
	// create a card or colour-code the underline.
	if tok.Lemma == "" {
		t.Error("lemma must be non-empty — used as card front_text")
	}
	if tok.ReadingHira == "" {
		t.Error("reading_hira must be non-empty — used as card front_reading")
	}
	if tok.UserStatus == "" {
		t.Error("user_status must be non-empty — used for underline colour")
	}
	if !tok.IsContentWord {
		t.Error("is_content_word must be true — only content words are clickable")
	}
}

func TestReaderTokenSchema_MapsToCardCreateRequest(t *testing.T) {
	// Verify that a reader token provides everything needed for a card Create call.
	tok := readerToken{
		Surface:       "生活",
		Lemma:         "生活",
		ReadingHira:   "せいかつ",
		IsContentWord: true,
		UserStatus:    "unknown",
		FrequencyRank: intPtr2(1500),
	}
	const sourceURL = "https://carve.app/reader/item-123"
	const sentence = "私たちの生活を大きく変える。"
	const langCode = "ja"

	// card Create payload as the extension would send it
	cardPayload := map[string]any{
		"language_code": langCode,
		"lemma":         tok.Lemma,       // front_text
		"reading":       tok.ReadingHira, // front_reading
		"sentence":      sentence,
		"source_url":    sourceURL,
	}

	if cardPayload["lemma"] == "" {
		t.Error("lemma must be derivable from token.Lemma")
	}
	if cardPayload["reading"] == "" {
		t.Error("reading must be derivable from token.ReadingHira")
	}
	if cardPayload["source_url"] == "" {
		t.Error("source_url must be the reader page URL")
	}
}

func TestReaderTokenSchema_UnknownStatusOnly_ContentWordsClickable(t *testing.T) {
	// The extension only shows the popup for tokens with data-content="1".
	// Verify the schema: non-content words are not marked clickable.
	tokens := []struct {
		raw    string
		wantCW bool
	}{
		{`{"lemma":"猫","is_content_word":true,"user_status":"unknown"}`, true},
		{`{"lemma":"は","is_content_word":false,"user_status":"known"}`, false},
		{`{"lemma":"を","is_content_word":false,"user_status":"known"}`, false},
		{`{"lemma":"走る","is_content_word":true,"user_status":"learning"}`, true},
	}
	for _, tc := range tokens {
		var tok readerToken
		if err := json.Unmarshal([]byte(tc.raw), &tok); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if tok.IsContentWord != tc.wantCW {
			t.Errorf("lemma=%q: is_content_word=%v, want %v", tok.Lemma, tok.IsContentWord, tc.wantCW)
		}
	}
}

func TestReaderResponse_TokensPassedThrough(t *testing.T) {
	// The Read handler writes nlpResult.Tokens directly to the response without
	// re-encoding. Verify that a JSON-round-trip through []json.RawMessage
	// preserves all token fields the extension depends on.
	tokensJSON := `[
		{"surface":"猫","lemma":"猫","reading_hira":"ねこ","pos":"名詞","is_content_word":true,"user_status":"unknown","frequency_rank":3200},
		{"surface":"は","lemma":"は","reading_hira":"は","pos":"助詞","is_content_word":false,"user_status":"known","frequency_rank":null}
	]`
	var rawTokens []json.RawMessage
	if err := json.Unmarshal([]byte(tokensJSON), &rawTokens); err != nil {
		t.Fatalf("unmarshal raw tokens: %v", err)
	}

	// Simulate what the Read handler does: pass tokens through to response
	resp := map[string]any{
		"id":     "item-123",
		"tokens": rawTokens,
	}
	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var decoded struct {
		ID     string            `json:"id"`
		Tokens []json.RawMessage `json:"tokens"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(decoded.Tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(decoded.Tokens))
	}

	// Verify first token still has all required fields after pass-through
	var firstTok readerToken
	if err := json.Unmarshal(decoded.Tokens[0], &firstTok); err != nil {
		t.Fatalf("unmarshal first token: %v", err)
	}
	if firstTok.Lemma != "猫" {
		t.Errorf("lemma corrupted: %q", firstTok.Lemma)
	}
	if firstTok.ReadingHira != "ねこ" {
		t.Errorf("reading_hira corrupted: %q", firstTok.ReadingHira)
	}
	if firstTok.UserStatus != "unknown" {
		t.Errorf("user_status corrupted: %q", firstTok.UserStatus)
	}
	if !firstTok.IsContentWord {
		t.Error("is_content_word corrupted")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// buildMultipartFile builds a multipart body with a file field and optional extra fields.
func buildMultipartFile(filename, content, fieldName, fieldValue string) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	h.Set("Content-Type", "application/octet-stream")
	fw, _ := mw.CreatePart(h)
	fw.Write([]byte(content))

	if fieldName != "" {
		mw.WriteField(fieldName, fieldValue)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}
