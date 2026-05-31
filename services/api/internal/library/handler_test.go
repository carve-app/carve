package library

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

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

// TestImportFile_TxtType and TestImportFile_SrtType are integration tests
// that require a real DB. File-type validation is covered by TestImportFile_WrongFileType.

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
