package importer

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

// ── stripHTMLSimple ───────────────────────────────────────────────────────────

func TestStripHTMLSimple_RemovesTags(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hello", "hello"},
		{"<b>bold</b>", "bold"},
		{"<span class=\"x\">text</span>", "text"},
		{"word1 <br/> word2", "word1  word2"},
		{"", ""},
		{"<p></p>", ""},
		{"no tags at all", "no tags at all"},
		{"mixed <em>italic</em> text", "mixed italic text"},
	}
	for _, c := range cases {
		got := stripHTMLSimple(c.in)
		if got != c.want {
			t.Errorf("stripHTMLSimple(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── auth guards ───────────────────────────────────────────────────────────────

func newImporterHandler() *Handler {
	return &Handler{db: nil}
}

func authedCtx(r *http.Request) *http.Request {
	return r.WithContext(auth.ContextWithClaims(r.Context(), &auth.Claims{UserID: "user-test-001"}))
}

func TestImportAnki_NoAuth(t *testing.T) {
	h := newImporterHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/import/anki", nil)
	w := httptest.NewRecorder()
	h.ImportAnki(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestImportMigakuCSV_NoAuth(t *testing.T) {
	h := newImporterHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/import/migaku-csv", nil)
	w := httptest.NewRecorder()
	h.ImportMigakuCSV(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestImportYomitan_NoAuth(t *testing.T) {
	h := newImporterHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/import/yomitan", nil)
	w := httptest.NewRecorder()
	h.ImportYomitan(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestImportJPDBCSV_NoAuth(t *testing.T) {
	h := newImporterHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/import/jpdb-csv", nil)
	w := httptest.NewRecorder()
	h.ImportJPDBCSV(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── Anki validation ───────────────────────────────────────────────────────────

func TestImportAnki_WrongFileType(t *testing.T) {
	h := newImporterHandler()
	body, ct := buildFile("cards.csv", "word,reading", "language", "ja")
	req := httptest.NewRequest(http.MethodPost, "/v1/import/anki", body)
	req.Header.Set("Content-Type", ct)
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.ImportAnki(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-.apkg file, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), ".apkg") {
		t.Errorf("expected .apkg error, got: %s", w.Body.String())
	}
}

func TestImportAnki_InvalidZip(t *testing.T) {
	h := newImporterHandler()
	body, ct := buildFile("deck.apkg", "not a zip archive", "language", "ja")
	req := httptest.NewRequest(http.MethodPost, "/v1/import/anki", body)
	req.Header.Set("Content-Type", ct)
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.ImportAnki(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid zip, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid") {
		t.Errorf("expected 'invalid' in error, got: %s", w.Body.String())
	}
}

func TestImportAnki_ZipWithNoCollection(t *testing.T) {
	h := newImporterHandler()
	// Build a valid zip that doesn't contain collection.anki2
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, _ := zw.Create("README.txt")
	f.Write([]byte("no collection here"))
	zw.Close()

	body, ct := buildFile("deck.apkg", zipBuf.String(), "language", "ja")
	req := httptest.NewRequest(http.MethodPost, "/v1/import/anki", body)
	req.Header.Set("Content-Type", ct)
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.ImportAnki(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when no collection.anki2 in zip, got %d", w.Code)
	}
}

// ── Migaku CSV validation ─────────────────────────────────────────────────────

func TestImportMigakuCSV_MissingFile(t *testing.T) {
	h := newImporterHandler()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("language", "ja")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/import/migaku-csv", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.ImportMigakuCSV(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing file, got %d", w.Code)
	}
}

func TestImportMigakuCSV_EmptyLinesSkipped(t *testing.T) {
	// Rows with empty first field are skipped without DB calls.
	h := newImporterHandler()
	// Header + two rows with empty word field → all skipped, imported=0
	csvContent := "word,reading,definition,sentence\n,reading1,def1,sent1\n,reading2,def2,sent2\n"
	body, ct := buildFile("words.csv", csvContent, "language", "ja")
	req := httptest.NewRequest(http.MethodPost, "/v1/import/migaku-csv", body)
	req.Header.Set("Content-Type", ct)
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.ImportMigakuCSV(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("response not valid JSON: %s", w.Body.String())
	}
	if res["imported"] != float64(0) {
		t.Errorf("expected imported=0, got: %v", res["imported"])
	}
	if res["skipped"] != float64(2) {
		t.Errorf("expected skipped=2, got: %v", res["skipped"])
	}
}

// ── Yomitan validation ────────────────────────────────────────────────────────

func TestImportYomitan_InvalidZip(t *testing.T) {
	h := newImporterHandler()
	body, ct := buildFile("export.zip", "not a zip", "language", "ja")
	req := httptest.NewRequest(http.MethodPost, "/v1/import/yomitan", body)
	req.Header.Set("Content-Type", ct)
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.ImportYomitan(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid zip, got %d", w.Code)
	}
}

func TestImportYomitan_ZipWithNoTermBanks(t *testing.T) {
	// Valid zip but no term_bank_* files → no DB calls → returns 0 imported
	h := newImporterHandler()
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, _ := zw.Create("index.json")
	f.Write([]byte(`{"title":"test"}`))
	zw.Close()

	body, ct := buildFileBytes("export.zip", zipBuf.Bytes(), "language", "ja")
	req := httptest.NewRequest(http.MethodPost, "/v1/import/yomitan", body)
	req.Header.Set("Content-Type", ct)
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.ImportYomitan(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for zip with no term banks, got %d: %s", w.Code, w.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("response not valid JSON: %s", w.Body.String())
	}
	if res["imported"] != float64(0) {
		t.Errorf("expected imported=0, got: %v", res["imported"])
	}
	if res["type"] != "known_words" {
		t.Errorf("expected type=known_words, got: %v", res["type"])
	}
}

// ── JPDB CSV status mapping ───────────────────────────────────────────────────

func TestImportJPDB_ResponseShape(t *testing.T) {
	// We can't test DB inserts, but we can test the response shape on an
	// empty (header-only) CSV that won't trigger any DB calls.
	// For this test we use a nil-DB handler and verify we get type=known_words.
	// However, nil DB would panic. Use a valid but empty CSV via a mock:
	// Instead test through response fields by checking JSON on happy path
	// with a CSV that has a header and zero data rows — the handler will
	// try the first DB insert only when there are data rows.
	h := newImporterHandler()
	csvContent := "vocabulary,reading,status\n"
	body, ct := buildFile("words.csv", csvContent, "language", "ja")
	req := httptest.NewRequest(http.MethodPost, "/v1/import/jpdb-csv", body)
	req.Header.Set("Content-Type", ct)
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.ImportJPDBCSV(w, req)
	// Should succeed with 0 rows (no DB calls, since only a header row)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for empty CSV, got %d: %s", w.Code, w.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("response not valid JSON: %s", w.Body.String())
	}
	if res["type"] != "known_words" {
		t.Errorf("expected type=known_words, got: %v", res["type"])
	}
	if res["imported"] != float64(0) {
		t.Errorf("expected imported=0, got: %v", res["imported"])
	}
}

func TestImportMigakuCSV_HeaderOnlyNoDBCalls(t *testing.T) {
	// Header-only CSV → 0 rows inserted → no DB calls → 200 with 0 imported
	h := newImporterHandler()
	csvContent := "word,reading,definition,sentence\n"
	body, ct := buildFile("words.csv", csvContent, "language", "ja")
	req := httptest.NewRequest(http.MethodPost, "/v1/import/migaku-csv", body)
	req.Header.Set("Content-Type", ct)
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.ImportMigakuCSV(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for header-only CSV, got %d: %s", w.Code, w.Body.String())
	}
	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("response not valid JSON: %s", w.Body.String())
	}
	if res["imported"] != float64(0) {
		t.Errorf("expected 0 imported, got: %v", res["imported"])
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func buildFile(filename, content, fieldName, fieldValue string) (*bytes.Buffer, string) {
	return buildFileBytes(filename, []byte(content), fieldName, fieldValue)
}

func buildFileBytes(filename string, data []byte, fieldName, fieldValue string) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	h.Set("Content-Type", "application/octet-stream")
	fw, _ := mw.CreatePart(h)
	fw.Write(data)

	if fieldName != "" {
		mw.WriteField(fieldName, fieldValue)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}
