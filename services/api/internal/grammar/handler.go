// Package grammar persists the JLPT grammar patterns a user has marked as
// known. The catalog of detectable patterns lives in the NLP service and is
// proxied separately (GET /v1/nlp/grammar/patterns); this handler only owns the
// per-user known/unknown state stored in user_known_patterns.
package grammar

import (
	"encoding/json"
	"log/slog"
	"net/http"

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

// knownPatternBody is the request body for marking/unmarking a pattern.
type knownPatternBody struct {
	LanguageCode string `json:"language_code"`
	PatternID    string `json:"pattern_id"`
}

// GET /v1/grammar/known?language=ja
// Returns the ids of patterns the authenticated user has marked known for the
// requested language.
func (h *Handler) ListKnown(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	language := r.URL.Query().Get("language")
	if language == "" {
		language = "ja"
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT pattern_id FROM user_known_patterns
		 WHERE user_id = $1 AND language_code = $2
		 ORDER BY pattern_id`,
		claims.UserID, language,
	)
	if err != nil {
		slog.Error("list known patterns", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	patternIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			slog.Error("scan known pattern", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		patternIDs = append(patternIDs, id)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rows error listing known patterns", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"pattern_ids": patternIDs})
}

// POST /v1/grammar/known
// Body: {language_code, pattern_id}. Idempotent — marking an already-known
// pattern is a no-op and still returns 200.
func (h *Handler) MarkKnown(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req knownPatternBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.LanguageCode == "" || req.PatternID == "" {
		writeError(w, http.StatusBadRequest, "language_code and pattern_id are required")
		return
	}

	_, err := h.db.Exec(r.Context(),
		`INSERT INTO user_known_patterns (user_id, language_code, pattern_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, language_code, pattern_id) DO NOTHING`,
		claims.UserID, req.LanguageCode, req.PatternID,
	)
	if err != nil {
		slog.Error("mark known pattern", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"language_code": req.LanguageCode,
		"pattern_id":    req.PatternID,
		"known":         true,
	})
}

// DELETE /v1/grammar/known
// Body: {language_code, pattern_id}. Idempotent unmark.
func (h *Handler) UnmarkKnown(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req knownPatternBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.LanguageCode == "" || req.PatternID == "" {
		writeError(w, http.StatusBadRequest, "language_code and pattern_id are required")
		return
	}

	_, err := h.db.Exec(r.Context(),
		`DELETE FROM user_known_patterns
		 WHERE user_id = $1 AND language_code = $2 AND pattern_id = $3`,
		claims.UserID, req.LanguageCode, req.PatternID,
	)
	if err != nil {
		slog.Error("unmark known pattern", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"language_code": req.LanguageCode,
		"pattern_id":    req.PatternID,
		"known":         false,
	})
}
