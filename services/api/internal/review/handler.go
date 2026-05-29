package review

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

// GET /v1/review/due-count?language=ja
func (h *Handler) DueCount(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	language := r.URL.Query().Get("language")
	if language == "" {
		language = "ja"
	}

	var count int
	err := h.db.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM cards
		 WHERE user_id = $1
		   AND language_code = $2
		   AND deleted_at IS NULL
		   AND suspended = FALSE
		   AND buried = FALSE
		   AND (fsrs_state = 'new' OR fsrs_due <= now())`,
		claims.UserID, language,
	).Scan(&count)
	if err != nil {
		slog.Error("due count query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"due_count":     count,
		"language_code": language,
	})
}
