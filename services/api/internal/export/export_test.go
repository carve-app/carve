package export

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// ── CSV formatting (pure) ──────────────────────────────────────────────────

func TestCSVRecord_ColumnOrder(t *testing.T) {
	c := exportCardRow{
		FrontText:           "猫",
		Reading:             "ねこ",
		BackText:            "cat",
		Sentence:            "猫が好き",
		SubtitleTranslation: "I like cats",
		SourceURL:           "https://example.com/v?t=12",
	}
	got := csvRecord(c)
	want := []string{"猫", "ねこ", "cat", "猫が好き", "I like cats", "https://example.com/v?t=12"}
	if len(got) != len(csvHeader) {
		t.Fatalf("record length %d != header length %d", len(got), len(csvHeader))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("col %d (%s): got %q want %q", i, csvHeader[i], got[i], want[i])
		}
	}
}

// TestCSVQuoting verifies encoding/csv handles commas, quotes and newlines in
// card text per RFC 4180 when fed through csvRecord.
func TestCSVQuoting(t *testing.T) {
	c := exportCardRow{
		FrontText: `comma, here`,
		Reading:   `quote"inside`,
		BackText:  "line\nbreak",
		Sentence:  "plain",
	}
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	if err := cw.Write(csvHeader); err != nil {
		t.Fatal(err)
	}
	if err := cw.Write(csvRecord(c)); err != nil {
		t.Fatal(err)
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		t.Fatal(err)
	}

	// Round-trip parse it back and confirm the values survive intact.
	cr := csv.NewReader(strings.NewReader(buf.String()))
	recs, err := cr.ReadAll()
	if err != nil {
		t.Fatalf("re-parse failed: %v\noutput:\n%s", err, buf.String())
	}
	if len(recs) != 2 {
		t.Fatalf("expected header + 1 row, got %d records", len(recs))
	}
	row := recs[1]
	if row[0] != "comma, here" {
		t.Errorf("comma field corrupted: %q", row[0])
	}
	if row[1] != `quote"inside` {
		t.Errorf("quote field corrupted: %q", row[1])
	}
	if row[2] != "line\nbreak" {
		t.Errorf("newline field corrupted: %q", row[2])
	}
}

func TestExportFilename(t *testing.T) {
	if got := exportFilename("ja", "csv"); got != "carve-ja.csv" {
		t.Errorf("got %q", got)
	}
	if got := exportFilename("", "apkg"); got != "carve-all.apkg" {
		t.Errorf("blank language got %q", got)
	}
}

// ── Note rendering (pure) ──────────────────────────────────────────────────

func TestRenderNote_EscapesAndStructures(t *testing.T) {
	c := exportCardRow{
		FrontText:           "a<b>&",
		Reading:             "rdg",
		BackText:            "back",
		Sentence:            "sent<x>",
		SubtitleTranslation: "trans",
	}
	n := renderNote(c)
	// HTML special chars in the front text must be escaped.
	if strings.Contains(n.Front, "<b>") {
		t.Errorf("front not escaped: %q", n.Front)
	}
	if !strings.Contains(n.Front, "a&lt;b&gt;&amp;") {
		t.Errorf("front escaping wrong: %q", n.Front)
	}
	// Reading appended on the front.
	if !strings.Contains(n.Front, "rdg") {
		t.Errorf("reading missing: %q", n.Front)
	}
	// Sentence text escaped on the back.
	if strings.Contains(n.Back, "<x>") {
		t.Errorf("sentence not escaped: %q", n.Back)
	}
	if !strings.Contains(n.Back, "trans") {
		t.Errorf("translation missing on back: %q", n.Back)
	}
}

func TestRenderNote_OmitsEmptyParts(t *testing.T) {
	n := renderNote(exportCardRow{FrontText: "x", BackText: "y"})
	if strings.Contains(n.Front, "reading") {
		t.Errorf("empty reading should not appear: %q", n.Front)
	}
	if strings.Contains(n.Back, "sentence") || strings.Contains(n.Back, "translation") {
		t.Errorf("empty sentence/translation should not appear: %q", n.Back)
	}
}

// ── FSRS → Anki scheduling (pure) ──────────────────────────────────────────

func fptr(v float64) *float64 { return &v }

func TestSchedFromFSRS(t *testing.T) {
	crt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	t.Run("new card", func(t *testing.T) {
		s := schedFromFSRS(exportCardRow{FsrsState: "new"}, 5, crt, now)
		if s.Type != 0 || s.Queue != 0 {
			t.Errorf("new card type/queue = %d/%d, want 0/0", s.Type, s.Queue)
		}
		if s.Due != 5 {
			t.Errorf("new card due should be position 5, got %d", s.Due)
		}
		if s.IVL != 0 {
			t.Errorf("new card ivl should be 0, got %d", s.IVL)
		}
	})

	t.Run("review card", func(t *testing.T) {
		s := schedFromFSRS(exportCardRow{
			FsrsState: "review", Stability: fptr(42), Difficulty: fptr(4), Reps: 7, Lapses: 1,
		}, 0, crt, now)
		if s.Type != 2 || s.Queue != 2 {
			t.Errorf("review type/queue = %d/%d, want 2/2", s.Type, s.Queue)
		}
		if s.IVL != 42 {
			t.Errorf("ivl = %d, want 42", s.IVL)
		}
		if s.Factor != 2500 { // difficulty 4 → 2500
			t.Errorf("factor = %d, want 2500", s.Factor)
		}
		if s.Reps != 7 || s.Lapses != 1 {
			t.Errorf("reps/lapses lost: %d/%d", s.Reps, s.Lapses)
		}
		// Due day = (now + 42d) since crt. now is ~161 days after crt; +42 = 203.
		wantDue := int(now.Add(42*24*time.Hour).Sub(crt).Hours() / 24)
		if s.Due != wantDue {
			t.Errorf("due = %d, want %d", s.Due, wantDue)
		}
	})

	t.Run("review card min interval", func(t *testing.T) {
		s := schedFromFSRS(exportCardRow{FsrsState: "review", Stability: fptr(0.1)}, 0, crt, now)
		if s.IVL < 1 {
			t.Errorf("review ivl must be >=1, got %d", s.IVL)
		}
	})

	t.Run("factor floor", func(t *testing.T) {
		// Very high difficulty would push ease below 1300; must clamp.
		s := schedFromFSRS(exportCardRow{FsrsState: "review", Difficulty: fptr(10), Stability: fptr(1)}, 0, crt, now)
		if s.Factor < 1300 {
			t.Errorf("factor must clamp to >=1300, got %d", s.Factor)
		}
	})

	t.Run("suspended", func(t *testing.T) {
		s := schedFromFSRS(exportCardRow{FsrsState: "suspended", Stability: fptr(10)}, 0, crt, now)
		if s.Queue != -1 {
			t.Errorf("suspended queue = %d, want -1", s.Queue)
		}
	})
}

// ── .apkg builder: read back the collection and assert the Anki schema ──────

func TestBuildAPKG_RoundTrip(t *testing.T) {
	rows := []exportCardRow{
		{FrontText: "猫", Reading: "ねこ", BackText: "cat", Sentence: "猫が好き",
			SubtitleTranslation: "I like cats", FsrsState: "review",
			Stability: fptr(42), Difficulty: fptr(4), Reps: 7, Lapses: 1,
			FrontImageName: "carve-image-000.png", FrontAudioName: "carve-audio-001.webm"},
		{FrontText: "犬", BackText: "dog", FsrsState: "new"},
	}
	media := []apkgMedia{
		{Name: "carve-image-000.png", Data: []byte("png-proof")},
		{Name: "carve-audio-001.webm", Data: []byte("audio-proof")},
	}

	apkg, err := buildAPKG(rows, "Carve ja", media)
	if err != nil {
		t.Fatalf("buildAPKG: %v", err)
	}
	if len(apkg) == 0 {
		t.Fatal("empty apkg")
	}

	// Unzip and confirm the expected members exist.
	zr, err := zip.NewReader(bytes.NewReader(apkg), int64(len(apkg)))
	if err != nil {
		t.Fatalf("apkg is not a valid zip: %v", err)
	}
	var collBytes, mediaBytes []byte
	mediaFiles := make(map[string][]byte)
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		switch f.Name {
		case "collection.anki2":
			collBytes = b
		case "media":
			mediaBytes = b
		default:
			mediaFiles[f.Name] = b
		}
	}
	if len(collBytes) == 0 {
		t.Fatal("collection.anki2 missing from .apkg")
	}
	var mediaMap map[string]string
	if err := json.Unmarshal(mediaBytes, &mediaMap); err != nil {
		t.Fatalf("invalid media manifest: %v", err)
	}
	if mediaMap["0"] != "carve-image-000.png" || mediaMap["1"] != "carve-audio-001.webm" {
		t.Fatalf("unexpected media manifest: %#v", mediaMap)
	}
	if string(mediaFiles["0"]) != "png-proof" || string(mediaFiles["1"]) != "audio-proof" {
		t.Fatalf("media payloads not preserved: %#v", mediaFiles)
	}

	// Write the collection out and open it with the same driver the importer
	// uses, then assert the schema/rows are what Anki expects.
	tmp, err := os.CreateTemp("", "carve_apkg_readback_*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(collBytes); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	db, err := sql.Open("sqlite", tmp.Name())
	if err != nil {
		t.Fatalf("open readback db: %v", err)
	}
	defer db.Close()

	// col table: exactly one row, with non-empty models/decks JSON.
	var (
		crt           int64
		models, decks string
	)
	if err := db.QueryRow(`SELECT crt, models, decks FROM col LIMIT 1`).Scan(&crt, &models, &decks); err != nil {
		t.Fatalf("read col: %v", err)
	}
	if crt == 0 {
		t.Error("col.crt is zero")
	}
	if !strings.Contains(models, "Carve Basic") || !strings.Contains(models, `"Front"`) || !strings.Contains(models, `"Back"`) {
		t.Errorf("models JSON missing Carve Basic / Front / Back: %s", models)
	}
	if !strings.Contains(decks, "Carve ja") {
		t.Errorf("decks JSON missing deck name: %s", decks)
	}

	// notes table: two rows, fields separated by 0x1f, sfld is the front text.
	noteRows, err := db.Query(`SELECT flds, sfld, mid FROM notes ORDER BY id`)
	if err != nil {
		t.Fatalf("query notes: %v", err)
	}
	defer noteRows.Close()
	var notesSeen int
	for noteRows.Next() {
		var flds, sfld string
		var mid int64
		if err := noteRows.Scan(&flds, &sfld, &mid); err != nil {
			t.Fatal(err)
		}
		notesSeen++
		if notesSeen == 1 && (!strings.Contains(flds, `<img src="carve-image-000.png">`) || !strings.Contains(flds, `[sound:carve-audio-001.webm]`)) {
			t.Errorf("first note does not reference packaged media: %q", flds)
		}
		parts := strings.Split(flds, "\x1f")
		if len(parts) != 2 {
			t.Errorf("note flds should have 2 fields separated by 0x1f, got %d: %q", len(parts), flds)
		}
		if mid != ankiModelID {
			t.Errorf("note mid = %d, want %d", mid, ankiModelID)
		}
		if notesSeen == 1 {
			if sfld != "猫" {
				t.Errorf("first note sfld = %q, want 猫", sfld)
			}
			if !strings.Contains(parts[0], "ねこ") {
				t.Errorf("first note front missing reading: %q", parts[0])
			}
			if !strings.Contains(parts[1], "I like cats") {
				t.Errorf("first note back missing translation: %q", parts[1])
			}
		}
	}
	if notesSeen != 2 {
		t.Fatalf("expected 2 notes, got %d", notesSeen)
	}

	// cards table: two rows, did set to our deck, the review card preserves
	// ivl/factor/reps, the new card is type/queue 0.
	cardRows, err := db.Query(`SELECT nid, did, type, queue, ivl, factor, reps, lapses FROM cards ORDER BY id`)
	if err != nil {
		t.Fatalf("query cards: %v", err)
	}
	defer cardRows.Close()
	var cardsSeen int
	for cardRows.Next() {
		var nid, did, ctype, queue, ivl, factor, reps, lapses int64
		if err := cardRows.Scan(&nid, &did, &ctype, &queue, &ivl, &factor, &reps, &lapses); err != nil {
			t.Fatal(err)
		}
		cardsSeen++
		if did != ankiDeckID {
			t.Errorf("card did = %d, want %d", did, ankiDeckID)
		}
		if cardsSeen == 1 { // review card 猫
			if ctype != 2 || queue != 2 {
				t.Errorf("review card type/queue = %d/%d, want 2/2", ctype, queue)
			}
			if ivl != 42 {
				t.Errorf("review card ivl = %d, want 42", ivl)
			}
			if reps != 7 || lapses != 1 {
				t.Errorf("review card reps/lapses = %d/%d, want 7/1", reps, lapses)
			}
		}
		if cardsSeen == 2 { // new card 犬
			if ctype != 0 || queue != 0 {
				t.Errorf("new card type/queue = %d/%d, want 0/0", ctype, queue)
			}
		}
	}
	if cardsSeen != 2 {
		t.Fatalf("expected 2 cards, got %d", cardsSeen)
	}

	// revlog must exist and be empty.
	var revlogCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM revlog`).Scan(&revlogCount); err != nil {
		t.Fatalf("revlog table missing/unreadable: %v", err)
	}
	if revlogCount != 0 {
		t.Errorf("revlog should be empty, got %d rows", revlogCount)
	}
}

func TestBuildAPKG_Empty(t *testing.T) {
	// No cards: still a valid .apkg (Anki imports an empty deck fine).
	apkg, err := buildAPKG(nil, "Carve ja", nil)
	if err != nil {
		t.Fatalf("buildAPKG empty: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(apkg), int64(len(apkg)))
	if err != nil {
		t.Fatalf("empty apkg not a zip: %v", err)
	}
	var hasColl bool
	for _, f := range zr.File {
		if f.Name == "collection.anki2" {
			hasColl = true
		}
	}
	if !hasColl {
		t.Error("empty apkg missing collection.anki2")
	}
}
