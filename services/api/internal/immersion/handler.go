package immersion

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

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

var validSessionTypes = map[string]bool{
	"reading":   true,
	"listening": true,
	"video":     true,
	"writing":   true,
}

var supportedLanguages = map[string]bool{
	"ja": true, "zh-cn": true, "zh-tw": true, "ko": true, "en": true,
	"es": true, "de": true, "fr": true, "it": true, "pt": true, "vi": true,
}

// POST /v1/immersion
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		LanguageCode string    `json:"language_code"`
		SessionType  string    `json:"session_type"`
		DurationSec  int       `json:"duration_sec"`
		StartedAt    time.Time `json:"started_at"`
		URL          *string   `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if !supportedLanguages[req.LanguageCode] {
		writeError(w, http.StatusBadRequest, "unsupported language_code")
		return
	}
	if !validSessionTypes[req.SessionType] {
		writeError(w, http.StatusBadRequest, "session_type must be one of: reading, listening, video, writing")
		return
	}
	if req.DurationSec <= 0 || req.DurationSec >= 86400 {
		writeError(w, http.StatusBadRequest, "duration_sec must be between 1 and 86399")
		return
	}
	if req.StartedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "started_at is required")
		return
	}

	endedAt := req.StartedAt.Add(time.Duration(req.DurationSec) * time.Second)

	id := auth.NewID()
	_, err := h.db.Exec(r.Context(),
		`INSERT INTO immersion_sessions
		    (id, user_id, language_code, session_type, started_at, ended_at, duration_sec, source)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'extension')`,
		id, claims.UserID, req.LanguageCode, req.SessionType,
		req.StartedAt, endedAt, req.DurationSec,
	)
	if err != nil {
		slog.Error("create immersion session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{})
}
