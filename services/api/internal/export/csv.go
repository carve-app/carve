package export

import (
	"encoding/csv"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/carve-app/carve/services/api/internal/auth"
)

// exportCardRow is the flattened, presentation-ready view of a card used by
// both the CSV and .apkg exporters. Keeping a single shared row type means the
// two formats stay in lockstep as columns are added.
type exportCardRow struct {
	FrontText           string
	Reading             string // front_reading
	BackText            string
	Sentence            string
	SubtitleTranslation string
	SourceURL           string
	FrontAudioURL       string
	FrontImageURL       string
	BackAudioURL        string
	SentenceAudioURL    string

	// Generated archive names used only by the APKG renderer. Keeping URLs and
	// names separate lets CSV remain text-only while APKG embeds stored media.
	FrontAudioName    string
	FrontImageName    string
	BackAudioName     string
	SentenceAudioName string

	// Scheduling (used by .apkg only; CSV ignores it). Pointers so "never
	// reviewed" stays distinct from "reviewed, value is zero".
	FsrsState  string
	Stability  *float64
	Difficulty *float64
	Reps       int
	Lapses     int
}

// csvHeader is the column order emitted by the CSV exporter. It matches the
// columns documented for the public /v1/export/csv endpoint. Anki and Migaku
// both import header-mapped CSV, so the names double as field hints.
var csvHeader = []string{
	"front_text",
	"reading",
	"back_text",
	"sentence",
	"subtitle_translation",
	"source_url",
}

// csvRecord projects a card row into the CSV column order. Pure function so it
// is trivially table-testable without a database.
func csvRecord(c exportCardRow) []string {
	return []string{
		c.FrontText,
		c.Reading,
		c.BackText,
		c.Sentence,
		c.SubtitleTranslation,
		c.SourceURL,
	}
}

// GET /v1/export/csv?language=ja
// Streams a UTF-8 CSV attachment, one row per (non-deleted) card. This is the
// pragmatic, universally-importable format: Anki and Migaku both ingest CSV.
func (h *Handler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	language := r.URL.Query().Get("language")

	rows, err := h.loadCardRows(r.Context(), claims.UserID, language)
	if err != nil {
		slog.Error("export csv: load cards", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	filename := exportFilename(language, "csv")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)
	// Write header then stream each record. encoding/csv handles quoting of
	// embedded commas, quotes and newlines per RFC 4180.
	_ = cw.Write(csvHeader)
	for _, c := range rows {
		if err := cw.Write(csvRecord(c)); err != nil {
			// Response is already committed (200 + partial body); log and stop.
			slog.Error("export csv: write row", "error", err)
			break
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		slog.Error("export csv: flush", "error", err)
	}
}

// exportFilename builds a stable, language-tagged attachment name. A blank
// language yields "carve-all.<ext>".
func exportFilename(language, ext string) string {
	lang := language
	if lang == "" {
		lang = "all"
	}
	return fmt.Sprintf("carve-%s.%s", lang, ext)
}
