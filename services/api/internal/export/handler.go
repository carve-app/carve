package export

import (
	"encoding/json"
	"fmt"
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

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// GET /v1/export
// Returns a full JSON dump of the authenticated user's data.
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ctx := r.Context()
	userID := claims.UserID

	// ── User profile ──────────────────────────────────────────────────────────
	var profile struct {
		ID          string    `json:"id"`
		Email       string    `json:"email"`
		DisplayName string    `json:"display_name"`
		CreatedAt   time.Time `json:"created_at"`
	}
	h.db.QueryRow(ctx,
		`SELECT id, email, display_name, created_at FROM users WHERE id = $1`,
		userID,
	).Scan(&profile.ID, &profile.Email, &profile.DisplayName, &profile.CreatedAt)

	// ── Cards ─────────────────────────────────────────────────────────────────
	cardRows, err := h.db.Query(ctx,
		`SELECT id, language_code, front_text, COALESCE(back_text,''), sentence,
		        source_url, source_timestamp, fsrs_state,
		        fsrs_stability, fsrs_difficulty, fsrs_due,
		        fsrs_last_review, fsrs_reps, fsrs_lapses,
		        suspended, buried, created_at, updated_at
		 FROM cards
		 WHERE user_id = $1 AND deleted_at IS NULL
		 ORDER BY created_at`,
		userID,
	)
	if err != nil {
		slog.Error("export cards query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer cardRows.Close()

	type exportCard struct {
		ID           string     `json:"id"`
		LanguageCode string     `json:"language_code"`
		FrontText    string     `json:"front_text"`
		BackText     string     `json:"back_text"`
		Sentence     *string    `json:"sentence"`
		SourceURL    *string    `json:"source_url"`
		SourceTS     *float64   `json:"source_timestamp"`
		FsrsState    string     `json:"fsrs_state"`
		Stability    *float64   `json:"stability"`
		Difficulty   *float64   `json:"difficulty"`
		Due          *time.Time `json:"due"`
		LastReview   *time.Time `json:"last_review"`
		Reps         int        `json:"reps"`
		Lapses       int        `json:"lapses"`
		Suspended    bool       `json:"suspended"`
		Buried       bool       `json:"buried"`
		CreatedAt    time.Time  `json:"created_at"`
		UpdatedAt    time.Time  `json:"updated_at"`
	}
	var cards []exportCard
	for cardRows.Next() {
		var c exportCard
		if err := cardRows.Scan(
			&c.ID, &c.LanguageCode, &c.FrontText, &c.BackText,
			&c.Sentence, &c.SourceURL, &c.SourceTS,
			&c.FsrsState, &c.Stability, &c.Difficulty, &c.Due, &c.LastReview,
			&c.Reps, &c.Lapses, &c.Suspended, &c.Buried,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			slog.Error("scan export card", "error", err)
			continue
		}
		cards = append(cards, c)
	}

	// ── Review events ─────────────────────────────────────────────────────────
	eventRows, err := h.db.Query(ctx,
		`SELECT id, card_id, reviewed_at, rating, time_taken_ms,
		        stability_after, difficulty_after, due_after, retrievability_at_review
		 FROM review_events
		 WHERE user_id = $1
		 ORDER BY reviewed_at`,
		userID,
	)
	if err != nil {
		slog.Error("export events query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer eventRows.Close()

	type exportEvent struct {
		ID              string     `json:"id"`
		CardID          string     `json:"card_id"`
		ReviewedAt      time.Time  `json:"reviewed_at"`
		Rating          int        `json:"rating"`
		TimeTakenMS     *int       `json:"time_taken_ms"`
		StabilityAfter  *float64   `json:"stability_after"`
		DifficultyAfter *float64   `json:"difficulty_after"`
		DueAfter        *time.Time `json:"due_after"`
		Retrievability  *float64   `json:"retrievability_at_review"`
	}
	var events []exportEvent
	for eventRows.Next() {
		var e exportEvent
		if err := eventRows.Scan(
			&e.ID, &e.CardID, &e.ReviewedAt, &e.Rating, &e.TimeTakenMS,
			&e.StabilityAfter, &e.DifficultyAfter, &e.DueAfter, &e.Retrievability,
		); err != nil {
			continue
		}
		events = append(events, e)
	}

	// ── Immersion sessions ────────────────────────────────────────────────────
	immRows, err := h.db.Query(ctx,
		`SELECT id, language_code, session_type, started_at, ended_at,
		        duration_sec, words_mined, lookups, source
		 FROM immersion_sessions
		 WHERE user_id = $1
		 ORDER BY started_at`,
		userID,
	)
	if err != nil {
		slog.Error("export immersion query", "error", err)
	}
	type exportImmersion struct {
		ID           string     `json:"id"`
		LanguageCode string     `json:"language_code"`
		SessionType  string     `json:"session_type"`
		StartedAt    time.Time  `json:"started_at"`
		EndedAt      *time.Time `json:"ended_at"`
		DurationSec  *int       `json:"duration_sec"`
		WordsMined   int        `json:"words_mined"`
		Lookups      int        `json:"lookups"`
		Source       string     `json:"source"`
	}
	var immersion []exportImmersion
	if immRows != nil {
		defer immRows.Close()
		for immRows.Next() {
			var im exportImmersion
			if err := immRows.Scan(
				&im.ID, &im.LanguageCode, &im.SessionType,
				&im.StartedAt, &im.EndedAt, &im.DurationSec,
				&im.WordsMined, &im.Lookups, &im.Source,
			); err != nil {
				continue
			}
			immersion = append(immersion, im)
		}
	}

	export := map[string]any{
		"version":          "1",
		"exported_at":      time.Now().UTC(),
		"user":             profile,
		"cards":            cards,
		"review_events":    events,
		"immersion":        immersion,
	}

	filename := fmt.Sprintf("carve-export-%s.json", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(export)
}
