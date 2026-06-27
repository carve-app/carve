package importer

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

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

func TestImportJPDBCSV_RejectsOversizedRequest(t *testing.T) {
	h := newImporterHandler()
	body, ct := buildFile("words.csv", strings.Repeat("x", int(maxJPDBRequestBytes)), "language", "ja")
	req := httptest.NewRequest(http.MethodPost, "/v1/import/jpdb-csv", body)
	req.Header.Set("Content-Type", ct)
	req = authedCtx(req)
	w := httptest.NewRecorder()
	h.ImportJPDBCSV(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", w.Code, w.Body.String())
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

// ── Anki roundtrip ────────────────────────────────────────────────────────────

// buildAnkiPackage creates a valid .apkg zip in memory containing a SQLite
// collection.anki2 with the given notes. Each note has front\x1fback\x1fsentence.
func buildAnkiPackage(notes []ankiNote) ([]byte, error) {
	// Write SQLite to a temp file (modernc.org/sqlite requires file path).
	tmpPath := fmt.Sprintf("/tmp/test_anki_%d.db", time.Now().UnixNano())
	defer os.Remove(tmpPath)

	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE notes (id INTEGER PRIMARY KEY, flds TEXT, tags TEXT)`); err != nil {
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	stmt, err := tx.Prepare(`INSERT INTO notes (flds, tags) VALUES (?, ?)`)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	for _, n := range notes {
		flds := n.Front + "\x1f" + n.Back + "\x1f" + n.Sentence
		if _, err := stmt.Exec(flds, ""); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	db.Close()

	dbData, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, err
	}

	// Wrap in zip as collection.anki2
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, err := zw.Create("collection.anki2")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(dbData); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return zipBuf.Bytes(), nil
}

func TestParseAnkiPackage_RoundtripBasic(t *testing.T) {
	input := []ankiNote{
		{Front: "猫", Back: "cat", Sentence: "猫がいる。"},
		{Front: "犬", Back: "dog", Sentence: ""},
		{Front: "<b>花</b>", Back: "<em>flower</em>", Sentence: ""},
	}

	pkg, err := buildAnkiPackage(input)
	if err != nil {
		t.Fatalf("buildAnkiPackage: %v", err)
	}

	got, err := parseAnkiPackage(pkg)
	if err != nil {
		t.Fatalf("parseAnkiPackage: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 notes, got %d", len(got))
	}

	// HTML should be stripped from front/back
	if got[2].Front != "花" {
		t.Errorf("expected stripped front '花', got '%s'", got[2].Front)
	}
	if got[2].Back != "flower" {
		t.Errorf("expected stripped back 'flower', got '%s'", got[2].Back)
	}

	// Fields should match for clean input
	if got[0].Front != "猫" {
		t.Errorf("expected '猫', got '%s'", got[0].Front)
	}
	if got[0].Back != "cat" {
		t.Errorf("expected 'cat', got '%s'", got[0].Back)
	}
	if got[0].Sentence != "猫がいる。" {
		t.Errorf("expected '猫がいる。', got '%s'", got[0].Sentence)
	}
}

func TestParseAnkiPackage_PreservesReferencedMedia(t *testing.T) {
	base, err := buildAnkiPackage([]ankiNote{{
		Front: `<img src="picture.png">猫[sound:front.webm]`,
		Back:  `cat[sound:back.mp3]`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	baseZip, err := zip.NewReader(bytes.NewReader(base), int64(len(base)))
	if err != nil {
		t.Fatal(err)
	}
	var collection []byte
	for _, entry := range baseZip.File {
		if entry.Name == "collection.anki2" {
			rc, _ := entry.Open()
			collection, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for name, data := range map[string][]byte{
		"collection.anki2": collection,
		"0":                []byte("png-data"),
		"1":                []byte("front-audio"),
		"2":                []byte("back-audio"),
		"media":            []byte(`{"0":"picture.png","1":"front.webm","2":"back.mp3"}`),
	} {
		entry, _ := zw.Create(name)
		_, _ = entry.Write(data)
	}
	_ = zw.Close()

	notes, err := parseAnkiPackage(archive.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 {
		t.Fatalf("notes=%d, want 1", len(notes))
	}
	note := notes[0]
	if note.Front != "猫" || note.Back != "cat" {
		t.Fatalf("media markup leaked into text: front=%q back=%q", note.Front, note.Back)
	}
	if note.FrontImage == nil || string(note.FrontImage.Data) != "png-data" || note.FrontImage.ContentType != "image/png" {
		t.Fatalf("front image not preserved: %#v", note.FrontImage)
	}
	if note.FrontAudio == nil || string(note.FrontAudio.Data) != "front-audio" {
		t.Fatalf("front audio not preserved: %#v", note.FrontAudio)
	}
	if note.BackAudio == nil || string(note.BackAudio.Data) != "back-audio" {
		t.Fatalf("back audio not preserved: %#v", note.BackAudio)
	}
}

func TestParseAnkiPackage_EmptyFrontSkipped(t *testing.T) {
	input := []ankiNote{
		{Front: "", Back: "ignored"},
		{Front: "   ", Back: "also ignored"},
		{Front: "valid", Back: "kept"},
	}
	// Notes with empty front won't be in the DB since we insert Front directly;
	// however the stripHTMLSimple trims spaces, so "   " → "" → skipped.
	// Build notes as raw flds strings to test skip logic.
	pkg, err := buildAnkiPackage(input)
	if err != nil {
		t.Fatalf("buildAnkiPackage: %v", err)
	}
	got, err := parseAnkiPackage(pkg)
	if err != nil {
		t.Fatalf("parseAnkiPackage: %v", err)
	}
	// Empty and whitespace-only fronts should be skipped
	for _, n := range got {
		if n.Front == "" {
			t.Error("parseAnkiPackage returned note with empty front")
		}
	}
}

func TestParseAnkiPackage_10kNotes_Under1s(t *testing.T) {
	const N = 10_000
	notes := make([]ankiNote, N)
	for i := range notes {
		notes[i] = ankiNote{
			Front:    fmt.Sprintf("word%05d", i),
			Back:     fmt.Sprintf("definition %d", i),
			Sentence: fmt.Sprintf("Example sentence number %d in the test deck.", i),
		}
	}

	pkg, err := buildAnkiPackage(notes)
	if err != nil {
		t.Fatalf("buildAnkiPackage for 10k notes: %v", err)
	}

	start := time.Now()
	got, err := parseAnkiPackage(pkg)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("parseAnkiPackage 10k: %v", err)
	}
	if len(got) != N {
		t.Errorf("expected %d notes, got %d", N, len(got))
	}
	maxDuration := time.Second
	if raceDetectorEnabled {
		// The race detector instruments every memory access and is materially
		// slower than a production build. Keep a bounded regression check in
		// race-enabled CI without weakening the normal <1s requirement.
		maxDuration = 3 * time.Second
	}
	if elapsed > maxDuration {
		t.Errorf("parsing 10k notes took %v, want < %v", elapsed, maxDuration)
	}
	t.Logf("parsed %d notes in %v (%.0f notes/s)", N, elapsed, float64(N)/elapsed.Seconds())
}

func TestParseAnkiPackage_FieldSplitByUnitSeparator(t *testing.T) {
	// Verify that \x1f is the field separator and that the parser handles
	// notes with only one field (no separator) without crashing.
	notes := []ankiNote{
		{Front: "単語", Back: "", Sentence: ""},
	}
	pkg, err := buildAnkiPackage(notes)
	if err != nil {
		t.Fatalf("buildAnkiPackage: %v", err)
	}
	got, err := parseAnkiPackage(pkg)
	if err != nil {
		t.Fatalf("parseAnkiPackage: %v", err)
	}
	if len(got) != 1 || got[0].Front != "単語" {
		t.Errorf("unexpected result: %+v", got)
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
