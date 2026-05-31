package importer

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
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
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
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

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
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

	data, err := io.ReadAll(io.LimitReader(file, 200<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}

	// .apkg is a zip archive containing collection.anki2 (SQLite)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid .apkg archive")
		return
	}

	var dbData []byte
	for _, f := range zr.File {
		if f.Name == "collection.anki2" || f.Name == "collection.anki21" {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			dbData, _ = io.ReadAll(rc)
			rc.Close()
			break
		}
	}
	if len(dbData) == 0 {
		writeError(w, http.StatusBadRequest, "could not find collection database in .apkg")
		return
	}

	// Write SQLite DB to a temp file (go-sqlite3 requires file path).
	tmpFile := fmt.Sprintf("/tmp/anki_import_%s_%d.db", claims.UserID, time.Now().UnixNano())
	if err := writeFile(tmpFile, dbData); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer removeFile(tmpFile)

	db, err := sql.Open("sqlite", tmpFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not open collection database")
		return
	}
	defer db.Close()

	// Read notes (each note = one card worth of content).
	rows, err := db.Query(`SELECT flds, tags FROM notes LIMIT 10000`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read cards from collection")
		return
	}
	defer rows.Close()

	ctx := r.Context()
	imported := 0
	skipped := 0

	for rows.Next() {
		var flds, tags string
		if err := rows.Scan(&flds, &tags); err != nil {
			continue
		}

		// Anki field separator is \x1f (Unit Separator).
		fields := strings.Split(flds, "\x1f")
		if len(fields) == 0 {
			skipped++
			continue
		}

		front := stripHTMLSimple(fields[0])
		if front == "" {
			skipped++
			continue
		}

		var backPtr *string
		if len(fields) > 1 {
			back := stripHTMLSimple(fields[1])
			if back != "" {
				backPtr = &back
			}
		}
		var sentencePtr *string
		if len(fields) > 2 {
			sentence := stripHTMLSimple(fields[2])
			if sentence != "" {
				sentencePtr = &sentence
			}
		}

		id := auth.NewID()
		_, err := h.db.Exec(ctx,
			`INSERT INTO cards
			    (id, user_id, language_code, front_text, back_text, sentence, fsrs_state)
			 VALUES ($1, $2, $3, $4, $5, $6, 'new')
			 ON CONFLICT DO NOTHING`,
			id, claims.UserID, language, front, backPtr, sentencePtr,
		)
		if err != nil {
			slog.Warn("anki import: insert card", "error", err, "front", front)
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

// ── POST /v1/import/migaku-csv ────────────────────────────────────────────────
// Migaku CSV export format: word,reading,definition,sentence,status

func (h *Handler) ImportMigakuCSV(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
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

	cr := csv.NewReader(io.LimitReader(file, 20<<20))
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
			s := record[1]; reading = &s
		}
		if len(record) > 2 && record[2] != "" {
			s := record[2]; definition = &s
		}
		if len(record) > 3 && record[3] != "" {
			s := record[3]; sentence = &s
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

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
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
	data, err := io.ReadAll(io.LimitReader(file, 50<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
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

	for _, f := range zr.File {
		if !strings.HasPrefix(filepath.Base(f.Name), "term_bank_") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}

		var entries [][]json.RawMessage
		if err := json.NewDecoder(rc).Decode(&entries); err != nil {
			rc.Close()
			continue
		}
		rc.Close()

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

			// Yomitan import: just mark as known in user_word_knowledge, no card.
			_, err := h.db.Exec(ctx,
				`INSERT INTO user_word_knowledge (user_id, language_code, lemma, status)
				 VALUES ($1, $2, $3, 'known')
				 ON CONFLICT (user_id, language_code, lemma)
				 DO UPDATE SET status = 'known', updated_at = now()`,
				claims.UserID, language, term,
			)
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

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
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

	cr := csv.NewReader(io.LimitReader(file, 10<<20))
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

		_, err := h.db.Exec(ctx,
			`INSERT INTO user_word_knowledge (user_id, language_code, lemma, status)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (user_id, language_code, lemma)
			 DO UPDATE SET status = EXCLUDED.status, updated_at = now()`,
			claims.UserID, language, word, status,
		)
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

func stripHTMLSimple(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' { inTag = true; continue }
		if r == '>' { inTag = false; continue }
		if !inTag { sb.WriteRune(r) }
	}
	return strings.TrimSpace(sb.String())
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0600)
}

func removeFile(path string) {
	os.Remove(path)
}
