package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db    *pgxpool.Pool
	media MediaUploader
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return NewHandlerWithMedia(db, newHTTPMediaUploader())
}

func NewHandlerWithMedia(db *pgxpool.Pool, media MediaUploader) *Handler {
	return &Handler{db: db, media: media}
}

const (
	maxAnkiRequestBytes    int64 = 101 << 20
	maxMigakuRequestBytes  int64 = 21 << 20
	maxYomitanRequestBytes int64 = 51 << 20
	maxJPDBRequestBytes    int64 = 11 << 20
)

func parseMultipartLimited(w http.ResponseWriter, r *http.Request, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	err := r.ParseMultipartForm(32 << 20)
	if err == nil {
		return nil
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) || strings.Contains(err.Error(), "request body too large") {
		writeError(w, http.StatusRequestEntityTooLarge, "upload too large")
		return err
	}
	writeError(w, http.StatusBadRequest, "invalid multipart form")
	return err
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ── POST /v1/import/anki ──────────────────────────────────────────────────────
// Accepts a .apkg file. Parses the embedded SQLite database and imports cards.

func (h *Handler) ImportAnki(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := parseMultipartLimited(w, r, maxAnkiRequestBytes); err != nil {
		return
	}

	language := r.FormValue("language")
	if language == "" {
		language = "ja"
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".apkg") {
		writeError(w, http.StatusBadRequest, "file must be .apkg")
		return
	}

	data, err := io.ReadAll(io.LimitReader(file, (100<<20)+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}
	if len(data) > 100<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "upload too large")
		return
	}

	notes, err := parseAnkiPackage(data)
	if err != nil {
		if strings.Contains(err.Error(), "size limit") || strings.Contains(err.Error(), "exceeds") {
			writeError(w, http.StatusRequestEntityTooLarge, "Anki package too large")
		} else if strings.Contains(err.Error(), "invalid .apkg") || strings.Contains(err.Error(), "media manifest") {
			writeError(w, http.StatusBadRequest, "invalid .apkg archive")
		} else if strings.Contains(err.Error(), "no collection") {
			writeError(w, http.StatusBadRequest, "could not find collection database in .apkg")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	ctx := r.Context()
	type preparedNote struct {
		note                                             ankiNote
		frontAudio, frontImage, backAudio, sentenceAudio *string
	}
	prepared := make([]preparedNote, 0, len(notes))
	uploadedMedia := make(map[string]string)
	for _, note := range notes {
		item := preparedNote{note: note}
		for _, media := range []struct {
			object *ankiMedia
			path   string
			dest   **string
		}{
			{note.FrontAudio, "/audio", &item.frontAudio},
			{note.FrontImage, "/screenshots", &item.frontImage},
			{note.BackAudio, "/audio", &item.backAudio},
			{note.SentenceAudio, "/audio", &item.sentenceAudio},
		} {
			if media.object == nil {
				continue
			}
			if existingURL, ok := uploadedMedia[media.object.Name]; ok {
				*media.dest = &existingURL
				continue
			}
			url, err := h.mediaUploader().Upload(ctx, media.path, media.object.Data, media.object.ContentType)
			if err != nil {
				slog.Warn("anki import: media upload", "error", err, "name", media.object.Name)
				writeError(w, http.StatusBadGateway, "could not store Anki media")
				return
			}
			uploadedMedia[media.object.Name] = url
			*media.dest = &url
		}
		prepared = append(prepared, item)
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback(ctx)
	imported := 0
	skipped := 0

	now := time.Now()
	for _, item := range prepared {
		note := item.note
		var backPtr *string
		if note.Back != "" {
			backPtr = &note.Back
		}
		var sentencePtr *string
		if note.Sentence != "" {
			sentencePtr = &note.Sentence
		}

		sched := Map(note.Sched, note.CollectionCreated, now)

		id := auth.NewID()
		result, err := tx.Exec(ctx,
			`INSERT INTO cards
			    (id, user_id, language_code, front_text, back_text, sentence,
			     front_audio_url, front_image_url, back_audio_url, sentence_audio_url,
			     fsrs_state, fsrs_stability, fsrs_difficulty, fsrs_due,
			     fsrs_reps, fsrs_lapses)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			 ON CONFLICT DO NOTHING`,
			id, claims.UserID, language, note.Front, backPtr, sentencePtr,
			item.frontAudio, item.frontImage, item.backAudio, item.sentenceAudio,
			sched.State, sched.Stability, sched.Difficulty, sched.Due,
			sched.Reps, sched.Lapses,
		)
		if err != nil {
			slog.Warn("anki import: insert card", "error", err, "front", note.Front)
			skipped++
			continue
		}
		if result.RowsAffected() == 0 {
			skipped++
		} else {
			imported++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"language": language,
	})
}

// ── POST /v1/import/migaku-csv ────────────────────────────────────────────────
// Migaku CSV export format: word,reading,definition,sentence,status

func (h *Handler) ImportMigakuCSV(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := parseMultipartLimited(w, r, maxMigakuRequestBytes); err != nil {
		return
	}

	language := r.FormValue("language")
	if language == "" {
		language = "ja"
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	csvData, err := io.ReadAll(io.LimitReader(file, (20<<20)+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}
	if len(csvData) > 20<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "upload too large")
		return
	}
	cr := csv.NewReader(bytes.NewReader(csvData))
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1 // variable fields

	records, err := cr.ReadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid CSV file")
		return
	}

	ctx := r.Context()
	imported := 0
	skipped := 0

	for i, record := range records {
		if i == 0 && len(record) > 0 && strings.EqualFold(record[0], "word") {
			continue // skip header
		}
		if len(record) < 1 || strings.TrimSpace(record[0]) == "" {
			skipped++
			continue
		}

		word := strings.TrimSpace(record[0])
		var reading, definition, sentence *string
		if len(record) > 1 && record[1] != "" {
			s := record[1]
			reading = &s
		}
		if len(record) > 2 && record[2] != "" {
			s := record[2]
			definition = &s
		}
		if len(record) > 3 && record[3] != "" {
			s := record[3]
			sentence = &s
		}

		id := auth.NewID()
		_, err := h.db.Exec(ctx,
			`INSERT INTO cards
			    (id, user_id, language_code, front_text, front_reading, back_text, sentence, fsrs_state)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, 'new')
			 ON CONFLICT DO NOTHING`,
			id, claims.UserID, language, word, reading, definition, sentence,
		)
		if err != nil {
			slog.Warn("migaku import: insert card", "error", err, "word", word)
			skipped++
			continue
		}
		imported++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"language": language,
	})
}

// ── POST /v1/import/yomitan ───────────────────────────────────────────────────
// Yomitan dictionary export format: array of [term, reading, definition_tags, rules, score, definitions, ...]

func (h *Handler) ImportYomitan(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := parseMultipartLimited(w, r, maxYomitanRequestBytes); err != nil {
		return
	}

	language := r.FormValue("language")
	if language == "" {
		language = "ja"
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	// Yomitan exports as a zip file containing term_bank_*.json files.
	data, err := io.ReadAll(io.LimitReader(file, (50<<20)+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}
	if len(data) > 50<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "upload too large")
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid Yomitan file (expected .zip)")
		return
	}

	ctx := r.Context()
	imported := 0
	skipped := 0

	var uncompressedTermBytes uint64
	for _, f := range zr.File {
		if !strings.HasPrefix(filepath.Base(f.Name), "term_bank_") {
			continue
		}
		if f.UncompressedSize64 > 50<<20 {
			writeError(w, http.StatusRequestEntityTooLarge, "archive entry too large")
			return
		}
		uncompressedTermBytes += f.UncompressedSize64
		if uncompressedTermBytes > 50<<20 {
			writeError(w, http.StatusRequestEntityTooLarge, "archive contents too large")
			return
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}

		entryData, readErr := io.ReadAll(io.LimitReader(rc, (50<<20)+1))
		rc.Close()
		if readErr != nil || len(entryData) > 50<<20 {
			writeError(w, http.StatusRequestEntityTooLarge, "archive entry too large")
			return
		}
		var entries [][]json.RawMessage
		if err := json.Unmarshal(entryData, &entries); err != nil {
			continue
		}

		for _, entry := range entries {
			if len(entry) < 2 {
				skipped++
				continue
			}

			var term, reading string
			json.Unmarshal(entry[0], &term)
			json.Unmarshal(entry[1], &reading)

			if term == "" {
				skipped++
				continue
			}

			// Yomitan imports known vocabulary rather than flashcards. Knowledge is
			// keyed through words.word_id; user_word_knowledge deliberately does not
			// duplicate language/lemma columns.
			err := h.upsertKnowledge(ctx, claims.UserID, language, term, reading, "known")
			if err != nil {
				slog.Warn("yomitan import: upsert knowledge", "error", err, "term", term)
				skipped++
				continue
			}
			imported++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"language": language,
		"type":     "known_words",
	})
}

// ── POST /v1/import/jpdb-csv ──────────────────────────────────────────────────
// JPDB known-words CSV export: vocabulary,reading,status

func (h *Handler) ImportJPDBCSV(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := parseMultipartLimited(w, r, maxJPDBRequestBytes); err != nil {
		return
	}

	language := r.FormValue("language")
	if language == "" {
		language = "ja"
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	csvData, err := io.ReadAll(io.LimitReader(file, (10<<20)+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}
	if len(csvData) > 10<<20 {
		writeError(w, http.StatusRequestEntityTooLarge, "upload too large")
		return
	}
	cr := csv.NewReader(bytes.NewReader(csvData))
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1

	records, err := cr.ReadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid CSV")
		return
	}

	ctx := r.Context()
	imported := 0
	skipped := 0

	for i, record := range records {
		if i == 0 {
			continue // skip header
		}
		if len(record) < 1 || strings.TrimSpace(record[0]) == "" {
			skipped++
			continue
		}

		word := strings.TrimSpace(record[0])
		status := "known"
		if len(record) >= 3 {
			switch strings.ToLower(strings.TrimSpace(record[2])) {
			case "learning", "reviewing":
				status = "learning"
			case "known", "never-forget":
				status = "known"
			default:
				status = "known"
			}
		}

		reading := ""
		if len(record) >= 2 {
			reading = strings.TrimSpace(record[1])
		}

		err := h.upsertKnowledge(ctx, claims.UserID, language, word, reading, status)
		if err != nil {
			skipped++
			continue
		}
		imported++
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"language": language,
		"type":     "known_words",
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (h *Handler) upsertKnowledge(
	ctx context.Context,
	userID string,
	language string,
	lemma string,
	reading string,
	status string,
) error {
	if reading == "" {
		reading = lemma
	}

	var wordID string
	if err := h.db.QueryRow(ctx,
		`INSERT INTO words (language_code, lemma, reading)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (language_code, lemma, reading)
		 DO UPDATE SET language_code = EXCLUDED.language_code
		 RETURNING id`,
		language, lemma, reading,
	).Scan(&wordID); err != nil {
		return err
	}

	_, err := h.db.Exec(ctx,
		`INSERT INTO user_word_knowledge
		    (user_id, word_id, status, first_seen_at, known_since)
		 VALUES ($1, $2, $3, now(), CASE WHEN $3 = 'known' THEN now() ELSE NULL END)
		 ON CONFLICT (user_id, word_id) DO UPDATE
		 SET status = EXCLUDED.status,
		     known_since = CASE
		         WHEN EXCLUDED.status = 'known'
		         THEN COALESCE(user_word_knowledge.known_since, now())
		         ELSE user_word_knowledge.known_since
		     END,
		     updated_at = now()`,
		userID, wordID, status,
	)
	return err
}

// ankiNote holds the parsed fields from a single Anki note row, plus the
// scheduling fields of its most-progressed sibling card. Anki cards table
// fields preserved: type, queue, ivl, factor, reps, lapses, due. Together
// they reconstruct the FSRS state via anki_sched.Map.
type ankiNote struct {
	Front         string
	Back          string
	Sentence      string
	FrontAudio    *ankiMedia
	FrontImage    *ankiMedia
	BackAudio     *ankiMedia
	SentenceAudio *ankiMedia

	// Scheduling fields from the most-progressed sibling card (max(type)).
	// Zero values mean "new card / no review history."
	Sched             AnkiCardSched
	CollectionCreated time.Time
}

// parseAnkiPackage extracts notes from an .apkg byte slice without touching the DB.
// It writes the embedded SQLite to a temp file, queries notes + cards, and returns them.
func parseAnkiPackage(data []byte) ([]ankiNote, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid .apkg archive: %w", err)
	}

	var dbData []byte
	var mediaManifest []byte
	archiveEntries := make(map[string]*zip.File)
	for _, f := range zr.File {
		archiveEntries[f.Name] = f
		if f.Name == "collection.anki2" || f.Name == "collection.anki21" {
			if f.UncompressedSize64 > 100<<20 {
				return nil, fmt.Errorf("collection database exceeds size limit")
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			dbData, _ = io.ReadAll(io.LimitReader(rc, (100<<20)+1))
			rc.Close()
		}
		if f.Name == "media" {
			if f.UncompressedSize64 > 1<<20 {
				return nil, fmt.Errorf("media manifest exceeds size limit")
			}
			rc, err := f.Open()
			if err == nil {
				mediaManifest, _ = io.ReadAll(io.LimitReader(rc, (1<<20)+1))
				rc.Close()
			}
		}
	}
	if len(dbData) == 0 {
		return nil, fmt.Errorf("no collection database found in .apkg")
	}
	if len(dbData) > 100<<20 {
		return nil, fmt.Errorf("collection database exceeds size limit")
	}
	if len(mediaManifest) > 1<<20 {
		return nil, fmt.Errorf("media manifest exceeds size limit")
	}
	mediaByName, err := readAnkiMedia(archiveEntries, mediaManifest)
	if err != nil {
		return nil, err
	}

	tmpFile := fmt.Sprintf("/tmp/anki_parse_%d.db", time.Now().UnixNano())
	if err := writeFile(tmpFile, dbData); err != nil {
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	defer removeFile(tmpFile)

	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	// Read collection creation time so day-relative `due` values can be
	// converted to absolute timestamps later.
	var crt int64
	_ = db.QueryRow(`SELECT crt FROM col LIMIT 1`).Scan(&crt)
	if crt == 0 {
		crt = time.Now().Unix() // fallback: today
	}
	collectionCreated := time.Unix(crt, 0)

	// Detect whether the .apkg includes a `cards` table. Some legacy
	// fixtures and bare exports don't include it; we still want to import
	// the notes (every card becomes 'new' as a safe fallback).
	hasCards := tableExists(db, "cards")

	var q string
	if hasCards {
		// Join notes with the most-progressed card per note. "Most progressed"
		// = highest type, then highest reps. Notes with reverse-sibling cards
		// import based on the more-studied sibling.
		q = `
			SELECT n.flds, n.tags,
			       COALESCE(c.type, 0)   AS c_type,
			       COALESCE(c.queue, 0)  AS c_queue,
			       COALESCE(c.ivl, 0)    AS c_ivl,
			       COALESCE(c.factor, 0) AS c_factor,
			       COALESCE(c.reps, 0)   AS c_reps,
			       COALESCE(c.lapses, 0) AS c_lapses,
			       COALESCE(c.due, 0)    AS c_due
			FROM notes n
			LEFT JOIN cards c ON c.id = (
			    SELECT id FROM cards
			    WHERE nid = n.id
			    ORDER BY type DESC, reps DESC, id ASC
			    LIMIT 1
			)
			LIMIT 10000
		`
	} else {
		q = `SELECT flds, tags, 0,0,0,0,0,0,0 FROM notes LIMIT 10000`
	}

	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("query notes: %w", err)
	}
	defer rows.Close()

	var notes []ankiNote
	for rows.Next() {
		var flds, tags string
		var cType, cQueue, cIVL, cFactor, cReps, cLapses int
		var cDue int64
		if err := rows.Scan(&flds, &tags, &cType, &cQueue, &cIVL, &cFactor, &cReps, &cLapses, &cDue); err != nil {
			continue
		}
		fields := strings.Split(flds, "\x1f")
		if len(fields) == 0 {
			continue
		}
		frontHTML := fields[0]
		front := stripHTMLSimple(stripSoundMarkup(frontHTML))
		if front == "" {
			continue
		}
		var back, sentence string
		backHTML := ""
		if len(fields) > 1 {
			backHTML = fields[1]
			back = stripHTMLSimple(stripSoundMarkup(backHTML))
		}
		if len(fields) > 2 {
			sentence = stripHTMLSimple(fields[2])
		}
		frontSounds := soundNames(frontHTML)
		backSounds := soundNames(backHTML)
		note := ankiNote{
			Front:    front,
			Back:     back,
			Sentence: sentence,
			Sched: AnkiCardSched{
				Type: cType, Queue: cQueue,
				IVL: cIVL, Factor: cFactor,
				Reps: cReps, Lapses: cLapses,
				Due: cDue,
			},
			CollectionCreated: collectionCreated,
		}
		if name := imageName(frontHTML); name != "" {
			note.FrontImage = mediaByName[name]
		}
		if len(frontSounds) > 0 {
			note.FrontAudio = mediaByName[frontSounds[0]]
		}
		if len(backSounds) > 0 {
			note.BackAudio = mediaByName[backSounds[0]]
		}
		if len(backSounds) > 1 {
			note.SentenceAudio = mediaByName[backSounds[1]]
		}
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func stripHTMLSimple(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			sb.WriteRune(r)
		}
	}
	return strings.TrimSpace(sb.String())
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0600)
}

func tableExists(db *sql.DB, name string) bool {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n)
	return err == nil && n > 0
}

func removeFile(path string) {
	os.Remove(path)
}
