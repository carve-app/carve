package export

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"hash/crc32"
	"html"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, already used by the importer

	"github.com/carve-app/carve/services/api/internal/auth"
)

// ── .apkg export ───────────────────────────────────────────────────────────
//
// An Anki .apkg is a ZIP containing:
//   - collection.anki2 : a SQLite DB with Anki's col / notes / cards / revlog
//                        tables (we target the legacy "anki2" schema, which the
//                        widest range of Anki versions imports).
//   - media            : a JSON map "0":"file0",... of packaged media files.
//
// We ship a minimal, single-model ("Carve Basic", Front/Back) collection with
// one deck. Media (audio/images) is intentionally OUT of v1 — the high-value
// core is front/back text + sentence + translation. The `media` file is the
// empty object "{}".
//
// FSRS → Anki scheduling is approximate. Anki will reschedule on first sync,
// so we only need values that are internally consistent and won't surprise the
// importer (e.g. review cards get a positive interval and a sane ease factor).
// The mapping mirrors importer.Unmap so a Carve→Anki→Carve round-trip stays
// stable, but we re-derive it locally rather than importing the importer
// package (export must not depend on importer).

const (
	ankiModelID  = 1700000000001 // arbitrary stable model id (ms-epoch-ish)
	ankiDeckID   = 1700000000002 // arbitrary stable deck id
	ankiFieldSep = "\x1f"        // Anki joins note fields with US (0x1f)
)

// ankiNote is the rendered note content for a single card.
type ankiNote struct {
	Front string // HTML for the Front field
	Back  string // HTML for the Back field
}

// renderNote projects an exportCardRow into the Front/Back fields of the
// "Carve Basic" model. Pure function — table-tested directly.
//
//	Front: front_text  (+ reading on a second line, if present)
//	Back:  back_text   (+ sentence + subtitle_translation, each on its own
//	                     line, if present)
//
// All user text is HTML-escaped; structure is expressed with <br> and small
// <div> wrappers so it renders cleanly in Anki without being parsed back as
// markup.
func renderNote(c exportCardRow) ankiNote {
	var front strings.Builder
	front.WriteString(html.EscapeString(c.FrontText))
	if c.Reading != "" {
		front.WriteString(`<br><span class="reading">`)
		front.WriteString(html.EscapeString(c.Reading))
		front.WriteString(`</span>`)
	}

	var back strings.Builder
	back.WriteString(html.EscapeString(c.BackText))
	if c.Sentence != "" {
		back.WriteString(`<br><div class="sentence">`)
		back.WriteString(html.EscapeString(c.Sentence))
		back.WriteString(`</div>`)
	}
	if c.SubtitleTranslation != "" {
		back.WriteString(`<br><div class="translation">`)
		back.WriteString(html.EscapeString(c.SubtitleTranslation))
		back.WriteString(`</div>`)
	}

	return ankiNote{Front: front.String(), Back: back.String()}
}

// ankiCard holds the scheduling slice we WRITE into the cards table. It is the
// inverse of importer.AnkiCardSched and is derived from a card's FSRS state.
type ankiCard struct {
	Type   int // 0=new 1=learn 2=review 3=relearn
	Queue  int // 0=new 1=learn 2=review -1=suspended
	IVL    int // interval in days
	Factor int // ease × 1000
	Reps   int
	Lapses int
	Due    int // review: days since collectionCreated; new: position
}

// schedFromFSRS inverts the importer's SM-2→FSRS mapping. Anki reschedules on
// import, so the exact numbers matter less than internal consistency:
//   - review/suspended cards get IVL = round(stability) (>=1) and a due day
//     relative to the collection epoch.
//   - factor is derived from difficulty: ease = (4 - difficulty)*300 + 2500,
//     clamped to a sane floor of 1300.
//   - new cards carry only their queue position.
//
// Pure function; `position` is the card's index (used as the new-card due so
// Anki preserves authoring order). `collectionCreated` and `now` anchor the
// relative due-day computation.
func schedFromFSRS(c exportCardRow, position int, collectionCreated, now time.Time) ankiCard {
	out := ankiCard{Reps: c.Reps, Lapses: c.Lapses}

	switch c.FsrsState {
	case "learning":
		out.Type, out.Queue = 1, 1
	case "relearning":
		out.Type, out.Queue = 3, 1
	case "suspended":
		out.Type, out.Queue = 2, -1
	case "review":
		out.Type, out.Queue = 2, 2
	default: // "new" or unknown
		out.Type, out.Queue = 0, 0
		out.Due = position
		out.Factor = 2500
		return out
	}

	// Interval from stability (days).
	if c.Stability != nil {
		out.IVL = int(math.Round(*c.Stability))
	}
	if out.IVL < 1 && out.Type == 2 {
		out.IVL = 1
	}

	// Ease factor from difficulty.
	if c.Difficulty != nil {
		out.Factor = int(math.Round((4.0-*c.Difficulty)*300.0 + 2500.0))
	} else {
		out.Factor = 2500
	}
	if out.Factor < 1300 {
		out.Factor = 1300
	}

	// Due day for scheduled cards: today + interval, expressed as days since
	// the collection epoch (Anki's convention for review cards).
	if out.Type == 2 {
		dueTime := now.Add(time.Duration(out.IVL) * 24 * time.Hour)
		out.Due = int(dueTime.Sub(collectionCreated).Hours() / 24)
	} else {
		// learning/relearning: due "soon"; position is fine as Anki re-anchors.
		out.Due = position
	}
	return out
}

// fieldChecksum reproduces Anki's notes.csum: the first 8 hex digits of the
// SHA1 of the first (sort) field, as an integer. Anki uses it for duplicate
// detection. We approximate with a CRC32 of the stripped first field — Anki
// recomputes csum on import, so an approximation is acceptable and keeps us
// off a crypto dependency for a non-load-bearing column. (Documented divergence.)
func fieldChecksum(sortField string) int64 {
	return int64(crc32.ChecksumIEEE([]byte(sortField)))
}

// modelsJSON builds the col.models JSON for the single "Carve Basic" model.
func modelsJSON() string {
	mid := strconv.FormatInt(ankiModelID, 10)
	return `{"` + mid + `":{` +
		`"id":` + mid + `,` +
		`"name":"Carve Basic",` +
		`"type":0,` +
		`"mod":0,"usn":0,"sortf":0,"did":` + strconv.FormatInt(ankiDeckID, 10) + `,` +
		`"flds":[` +
		`{"name":"Front","ord":0,"sticky":false,"rtl":false,"font":"Arial","size":20},` +
		`{"name":"Back","ord":1,"sticky":false,"rtl":false,"font":"Arial","size":20}],` +
		`"tmpls":[{"name":"Card 1","ord":0,` +
		`"qfmt":"{{Front}}",` +
		`"afmt":"{{FrontSide}}<hr id=answer>{{Back}}",` +
		`"bqfmt":"","bafmt":"","did":null,"bfont":"","bsize":0}],` +
		`"css":".card{font-family:Arial;font-size:20px;text-align:center;color:black;background-color:white}",` +
		`"latexPre":"","latexPost":"","latexsvg":false,"req":[[0,"any",[0]]],"tags":[],"vers":[]}}`
}

// decksJSON builds the col.decks JSON: a default deck (id 1, required by Anki)
// plus our named "Carve <language>" deck.
func decksJSON(deckName string) string {
	did := strconv.FormatInt(ankiDeckID, 10)
	esc := func(s string) string {
		// JSON string escaping for the deck name (handles quotes/backslashes).
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		return s
	}
	deck := func(id, name string) string {
		return `"` + id + `":{"id":` + id + `,"name":"` + esc(name) + `",` +
			`"mod":0,"usn":0,"collapsed":false,"desc":"","dyn":0,` +
			`"conf":1,"extendNew":0,"extendRev":0,"newToday":[0,0],` +
			`"revToday":[0,0],"lrnToday":[0,0],"timeToday":[0,0]}`
	}
	return `{` + deck("1", "Default") + `,` + deck(did, deckName) + `}`
}

// confJSON / dconfJSON are minimal but valid collection/deck config blobs.
func confJSON() string {
	return `{"nextPos":1,"estTimes":true,"activeDecks":[1],"sortType":"noteFld",` +
		`"timeLim":0,"sortBackwards":false,"addToCur":true,"curDeck":` +
		strconv.FormatInt(ankiDeckID, 10) + `,"newBury":true,"newSpread":0,` +
		`"dueCounts":true,"curModel":"` + strconv.FormatInt(ankiModelID, 10) + `","collapseTime":1200}`
}

func dconfJSON() string {
	return `{"1":{"id":1,"name":"Default","mod":0,"usn":0,"maxTaken":60,"autoplay":true,` +
		`"timer":0,"replayq":true,"new":{"bury":true,"delays":[1,10],"initialFactor":2500,` +
		`"ints":[1,4,7],"order":1,"perDay":20},"rev":{"bury":true,"ease4":1.3,"ivlFct":1,` +
		`"maxIvl":36500,"perDay":200,"hardFactor":1.2},"lapse":{"delays":[10],"leechAction":1,` +
		`"leechFails":8,"minInt":1,"mult":0},"dyn":false}}`
}

// buildCollectionDB writes a complete Anki collection SQLite database to
// `path` (modernc.org/sqlite requires a file path, not :memory:). Returns
// after the DB is fully written and closed so the file can be read/zipped.
func buildCollectionDB(path, deckName string, rows []exportCardRow, collectionCreated, now time.Time) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	// Anki "anki2" schema. Columns match what the importer (and Anki itself)
	// reads; columns we don't populate get sensible defaults.
	schema := `
		CREATE TABLE col (
			id     INTEGER PRIMARY KEY,
			crt    INTEGER NOT NULL,
			mod    INTEGER NOT NULL,
			scm    INTEGER NOT NULL,
			ver    INTEGER NOT NULL,
			dty    INTEGER NOT NULL,
			usn    INTEGER NOT NULL,
			ls     INTEGER NOT NULL,
			conf   TEXT NOT NULL,
			models TEXT NOT NULL,
			decks  TEXT NOT NULL,
			dconf  TEXT NOT NULL,
			tags   TEXT NOT NULL
		);
		CREATE TABLE notes (
			id    INTEGER PRIMARY KEY,
			guid  TEXT NOT NULL,
			mid   INTEGER NOT NULL,
			mod   INTEGER NOT NULL,
			usn   INTEGER NOT NULL,
			tags  TEXT NOT NULL,
			flds  TEXT NOT NULL,
			sfld  TEXT NOT NULL,
			csum  INTEGER NOT NULL,
			flags INTEGER NOT NULL,
			data  TEXT NOT NULL
		);
		CREATE TABLE cards (
			id     INTEGER PRIMARY KEY,
			nid    INTEGER NOT NULL,
			did    INTEGER NOT NULL,
			ord    INTEGER NOT NULL,
			mod    INTEGER NOT NULL,
			usn    INTEGER NOT NULL,
			type   INTEGER NOT NULL,
			queue  INTEGER NOT NULL,
			due    INTEGER NOT NULL,
			ivl    INTEGER NOT NULL,
			factor INTEGER NOT NULL,
			reps   INTEGER NOT NULL,
			lapses INTEGER NOT NULL,
			left   INTEGER NOT NULL,
			odue   INTEGER NOT NULL,
			odid   INTEGER NOT NULL,
			flags  INTEGER NOT NULL,
			data   TEXT NOT NULL
		);
		CREATE TABLE revlog (
			id     INTEGER PRIMARY KEY,
			cid    INTEGER NOT NULL,
			usn    INTEGER NOT NULL,
			ease   INTEGER NOT NULL,
			ivl    INTEGER NOT NULL,
			lastIvl INTEGER NOT NULL,
			factor INTEGER NOT NULL,
			time   INTEGER NOT NULL,
			type   INTEGER NOT NULL
		);
		CREATE TABLE graves (usn INTEGER NOT NULL, oid INTEGER NOT NULL, type INTEGER NOT NULL);
		CREATE INDEX ix_notes_csum ON notes (csum);
		CREATE INDEX ix_cards_nid ON cards (nid);
		CREATE INDEX ix_cards_sched ON cards (did, queue, due);
		CREATE INDEX ix_revlog_cid ON revlog (cid);
	`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	nowMS := now.UnixMilli()
	if _, err := db.Exec(
		`INSERT INTO col (id, crt, mod, scm, ver, dty, usn, ls, conf, models, decks, dconf, tags)
		 VALUES (1, ?, ?, ?, 11, 0, 0, 0, ?, ?, ?, ?, '{}')`,
		collectionCreated.Unix(), nowMS, nowMS,
		confJSON(), modelsJSON(), decksJSON(deckName), dconfJSON(),
	); err != nil {
		return fmt.Errorf("insert col: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	noteStmt, err := tx.Prepare(
		`INSERT INTO notes (id, guid, mid, mod, usn, tags, flds, sfld, csum, flags, data)
		 VALUES (?, ?, ?, ?, -1, '', ?, ?, ?, 0, '')`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare note: %w", err)
	}
	cardStmt, err := tx.Prepare(
		`INSERT INTO cards (id, nid, did, ord, mod, usn, type, queue, due, ivl, factor, reps, lapses, left, odue, odid, flags, data)
		 VALUES (?, ?, ?, 0, ?, -1, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, 0, '')`)
	if err != nil {
		noteStmt.Close()
		tx.Rollback()
		return fmt.Errorf("prepare card: %w", err)
	}

	// Use millisecond-epoch ids spaced by index, matching Anki's id scheme
	// (ids are timestamps; uniqueness within the deck is all that matters).
	baseID := nowMS
	for i, c := range rows {
		note := renderNote(c)
		flds := note.Front + ankiFieldSep + note.Back
		noteID := baseID + int64(i)*2
		cardID := baseID + int64(i)*2 + 1
		guid := ankiGUID(noteID)

		if _, err := noteStmt.Exec(
			noteID, guid, int64(ankiModelID), now.Unix(),
			flds, c.FrontText, fieldChecksum(c.FrontText),
		); err != nil {
			noteStmt.Close()
			cardStmt.Close()
			tx.Rollback()
			return fmt.Errorf("insert note %d: %w", i, err)
		}

		sched := schedFromFSRS(c, i, collectionCreated, now)
		if _, err := cardStmt.Exec(
			cardID, noteID, int64(ankiDeckID), now.Unix(),
			sched.Type, sched.Queue, sched.Due, sched.IVL,
			sched.Factor, sched.Reps, sched.Lapses,
		); err != nil {
			noteStmt.Close()
			cardStmt.Close()
			tx.Rollback()
			return fmt.Errorf("insert card %d: %w", i, err)
		}
	}
	noteStmt.Close()
	cardStmt.Close()
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// ankiGUID derives a short stable globally-unique id for a note. Anki accepts
// any unique string; we base it on the note id so re-exports are stable.
func ankiGUID(noteID int64) string {
	return strconv.FormatInt(noteID, 36)
}

// buildAPKG assembles the full .apkg archive in memory: it writes the
// collection DB to a temp file, reads it back, and zips it together with an
// empty `media` map. Returns the .apkg bytes.
func buildAPKG(rows []exportCardRow, deckName string) ([]byte, error) {
	now := time.Now().UTC()
	// Collection epoch at local midnight (Anki convention); UTC is fine here
	// because the importer treats crt as a plain unix second anyway.
	collectionCreated := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	tmp := fmt.Sprintf("%s/carve_apkg_export_%d.db", os.TempDir(), time.Now().UnixNano())
	defer os.Remove(tmp)

	if err := buildCollectionDB(tmp, deckName, rows, collectionCreated, now); err != nil {
		return nil, err
	}

	dbBytes, err := os.ReadFile(tmp)
	if err != nil {
		return nil, fmt.Errorf("read collection: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// collection.anki2 — the legacy filename, importable by the widest range
	// of Anki versions (and by our own importer, which checks both names).
	cf, err := zw.Create("collection.anki2")
	if err != nil {
		return nil, fmt.Errorf("zip collection: %w", err)
	}
	if _, err := cf.Write(dbBytes); err != nil {
		return nil, fmt.Errorf("write collection: %w", err)
	}

	// media — empty mapping. Media (audio/images) is intentionally excluded
	// from v1; the empty object is required for a valid .apkg.
	mf, err := zw.Create("media")
	if err != nil {
		return nil, fmt.Errorf("zip media: %w", err)
	}
	if _, err := mf.Write([]byte("{}")); err != nil {
		return nil, fmt.Errorf("write media: %w", err)
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip: %w", err)
	}
	return buf.Bytes(), nil
}

// deckNameFor builds the user-facing deck name "Carve <language>".
func deckNameFor(language string) string {
	if language == "" {
		return "Carve"
	}
	return "Carve " + language
}

// GET /v1/export/apkg?language=ja
// Builds a minimal valid Anki .apkg and returns it as an attachment.
func (h *Handler) ExportAPKG(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	language := r.URL.Query().Get("language")

	rows, err := h.loadCardRows(r.Context(), claims.UserID, language)
	if err != nil {
		slog.Error("export apkg: load cards", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	data, err := buildAPKG(rows, deckNameFor(language))
	if err != nil {
		slog.Error("export apkg: build", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	filename := exportFilename(language, "apkg")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		slog.Error("export apkg: write response", "error", err)
	}
}
