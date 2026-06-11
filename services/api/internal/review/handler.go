package review

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/carve-app/carve/services/api/internal/fsrs"
	"github.com/carve-app/carve/services/api/internal/metrics"
	"github.com/go-chi/chi/v5"
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

// loadParams fetches a user's FSRS params (or returns defaults).
func (h *Handler) loadParams(ctx interface{ Value(any) any }, userID, lang string, db interface {
	QueryRow(context interface{ Value(any) any }, sql string, args ...any) interface {
		Scan(dest ...any) error
	}
}) fsrs.Params {
	return fsrs.DefaultParams()
}

// loadParamsDB fetches FSRS params for a user/language from DB.
func (h *Handler) loadParamsDB(r *http.Request, userID, lang string) fsrs.Params {
	p := fsrs.DefaultParams()
	var weightsJSON []byte
	var retention float64
	err := h.db.QueryRow(r.Context(),
		`SELECT weights, target_retention FROM user_fsrs_params
		 WHERE user_id = $1 AND language_code = $2`,
		userID, lang,
	).Scan(&weightsJSON, &retention)
	if err != nil {
		return p // default
	}
	var wSlice []float64
	if err := json.Unmarshal(weightsJSON, &wSlice); err == nil && len(wSlice) == 19 {
		for i := range wSlice {
			p.W[i] = wSlice[i]
		}
	}
	if retention > 0 && retention < 1 {
		p.TargetRetention = retention
	}
	return p
}

// ── GET /v1/review/due-count ──────────────────────────────────────────────────

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

// ── GET /v1/review/session ────────────────────────────────────────────────────

type sessionCard struct {
	ID         string     `json:"id"`
	FrontText  string     `json:"front_text"`
	CardType   string     `json:"card_type"`
	BackText   *string    `json:"back_text"`
	Sentence   *string    `json:"sentence"`
	SourceURL  *string    `json:"source_url"`
	AudioURL   *string    `json:"audio_url"`
	ImageURL   *string    `json:"image_url"`
	FsrsState  string     `json:"fsrs_state"`
	Stability  *float64   `json:"stability"`
	Difficulty *float64   `json:"difficulty"`
	Due        *time.Time `json:"due"`
	Reps       int        `json:"reps"`
	Lapses     int        `json:"lapses"`
}

func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := r.URL.Query()
	language := q.Get("language")
	if language == "" {
		language = "ja"
	}
	limit := 20
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	// Fetch all due cards, learning-priority first (smallest due time first)
	rows, err := h.db.Query(r.Context(),
		`SELECT id, front_text, card_type, back_text, sentence, source_url,
		        front_audio_url, front_image_url,
		        fsrs_state, fsrs_stability, fsrs_difficulty, fsrs_due,
		        fsrs_reps, fsrs_lapses
		 FROM cards
		 WHERE user_id = $1
		   AND language_code = $2
		   AND deleted_at IS NULL
		   AND suspended = FALSE
		   AND buried = FALSE
		   AND (fsrs_state = 'new' OR fsrs_due <= now())
		 ORDER BY
		   CASE fsrs_state
		     WHEN 'learning'   THEN 0
		     WHEN 'relearning' THEN 0
		     WHEN 'review'     THEN 1
		     WHEN 'new'        THEN 2
		   END,
		   fsrs_due ASC NULLS LAST
		 LIMIT $3`,
		claims.UserID, language, limit*3, // fetch 3× to allow desimilarization
	)
	if err != nil {
		slog.Error("session query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	type rawCard struct {
		sessionCard
		sortKey string // first rune of front_text for desimilarization
	}

	var all []rawCard
	for rows.Next() {
		var c rawCard
		if err := rows.Scan(
			&c.ID, &c.FrontText, &c.CardType, &c.BackText, &c.Sentence, &c.SourceURL,
			&c.AudioURL, &c.ImageURL,
			&c.FsrsState, &c.Stability, &c.Difficulty, &c.Due,
			&c.Reps, &c.Lapses,
		); err != nil {
			slog.Error("scan session row", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		// Use first rune as desimilarization key (anti-similarity bucket)
		r, _ := utf8.DecodeRuneInString(c.FrontText)
		c.sortKey = string(r)
		all = append(all, c)
	}
	if err := rows.Err(); err != nil {
		slog.Error("rows error session", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Desimilarizer: pick at most 2 cards per first-rune bucket to avoid
	// confusingly similar cards (e.g., 食べる / 食べない) in the same session.
	seen := make(map[string]int)
	var session []sessionCard
	for _, c := range all {
		if len(session) >= limit {
			break
		}
		const maxPerBucket = 2
		if seen[c.sortKey] < maxPerBucket {
			session = append(session, c.sessionCard)
			seen[c.sortKey]++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"cards":         session,
		"total":         len(session),
		"language_code": language,
	})
}

// ── POST /v1/review/events ────────────────────────────────────────────────────

func (h *Handler) SubmitEvent(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		CardID      string `json:"card_id"`
		Rating      int    `json:"rating"`
		TimeMS      *int   `json:"time_taken_ms"`
		ReviewedAt  string `json:"reviewed_at"` // ISO 8601; empty = now
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.CardID == "" {
		writeError(w, http.StatusBadRequest, "card_id required")
		return
	}
	if req.Rating < 1 || req.Rating > 4 {
		writeError(w, http.StatusBadRequest, "rating must be 1–4")
		return
	}

	reviewedAt := time.Now().UTC()
	if req.ReviewedAt != "" {
		if t, err := time.Parse(time.RFC3339, req.ReviewedAt); err == nil {
			reviewedAt = t.UTC()
		}
	}

	ctx := r.Context()

	// Fetch current card state (verify ownership).
	var card struct {
		Stability  *float64
		Difficulty *float64
		LastReview *time.Time
		State      string
		Reps       int
		Lapses     int
		Language   string
	}
	err := h.db.QueryRow(ctx,
		`SELECT fsrs_stability, fsrs_difficulty, fsrs_last_review,
		        fsrs_state, fsrs_reps, fsrs_lapses, language_code
		 FROM cards
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL AND suspended = FALSE`,
		req.CardID, claims.UserID,
	).Scan(
		&card.Stability, &card.Difficulty, &card.LastReview,
		&card.State, &card.Reps, &card.Lapses, &card.Language,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}

	p := h.loadParamsDB(r, claims.UserID, card.Language)

	cs := fsrs.CardState{
		State:      fsrs.State(card.State),
		Stability:  derefFloat(card.Stability),
		Difficulty: derefFloat(card.Difficulty),
		Reps:       card.Reps,
		Lapses:     card.Lapses,
	}
	if card.LastReview != nil {
		cs.LastReview = *card.LastReview
	}

	result := fsrs.Schedule(p, cs, fsrs.Rating(req.Rating), reviewedAt)
	metrics.IncCounter("fsrs_event_total")

	// Persist updated card state
	_, err = h.db.Exec(ctx,
		`UPDATE cards SET
		   fsrs_state      = $1,
		   fsrs_stability  = $2,
		   fsrs_difficulty = $3,
		   fsrs_due        = $4,
		   fsrs_last_review= $5,
		   fsrs_reps       = $6,
		   fsrs_lapses     = $7,
		   suspended       = $8
		 WHERE id = $9 AND user_id = $10`,
		string(result.State),
		result.Stability,
		result.Difficulty,
		result.Due,
		reviewedAt,
		result.Reps,
		result.Lapses,
		result.IsLeech, // suspend the card if it's a leech
		req.CardID, claims.UserID,
	)
	if err != nil {
		slog.Error("update card fsrs state", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Store review event with prior state snapshot (needed for undo).
	eventID := auth.NewID()
	_, err = h.db.Exec(ctx,
		`INSERT INTO review_events
		   (id, card_id, user_id, reviewed_at, rating, time_taken_ms,
		    stability_after, difficulty_after, due_after, retrievability_at_review,
		    prior_fsrs_state, prior_fsrs_stability, prior_fsrs_difficulty,
		    prior_fsrs_due, prior_fsrs_reps, prior_fsrs_lapses)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		eventID, req.CardID, claims.UserID, reviewedAt, req.Rating, req.TimeMS,
		result.Stability, result.Difficulty, result.Due, result.Retrievability,
		card.State, card.Stability, card.Difficulty, card.LastReview, card.Reps, card.Lapses,
	)
	if err != nil {
		slog.Error("insert review event", "error", err)
		// Non-fatal: card state is already updated
	}

	// Create leech notification
	if result.IsLeech {
		notifID := auth.NewID()
		payload, _ := json.Marshal(map[string]any{
			"card_id":   req.CardID,
			"front":     "", // front text not fetched here; UI shows it
			"lapses":    result.Lapses,
			"suspended": true,
		})
		_, _ = h.db.Exec(ctx,
			`INSERT INTO user_notifications (id, user_id, type, payload)
			 VALUES ($1,$2,'leech_suspended',$3)`,
			notifID, claims.UserID, payload,
		)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"state":      string(result.State),
		"stability":  result.Stability,
		"difficulty": result.Difficulty,
		"due":        result.Due,
		"reps":       result.Reps,
		"lapses":     result.Lapses,
		"is_leech":   result.IsLeech,
	})
}

// ── GET /v1/review/intervals?card_id=xxx ─────────────────────────────────────

func (h *Handler) Intervals(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cardID := r.URL.Query().Get("card_id")
	if cardID == "" {
		writeError(w, http.StatusBadRequest, "card_id required")
		return
	}

	var card struct {
		Stability  *float64
		Difficulty *float64
		LastReview *time.Time
		State      string
		Reps       int
		Lapses     int
		Language   string
	}
	err := h.db.QueryRow(r.Context(),
		`SELECT fsrs_stability, fsrs_difficulty, fsrs_last_review,
		        fsrs_state, fsrs_reps, fsrs_lapses, language_code
		 FROM cards WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		cardID, claims.UserID,
	).Scan(
		&card.Stability, &card.Difficulty, &card.LastReview,
		&card.State, &card.Reps, &card.Lapses, &card.Language,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}

	p := h.loadParamsDB(r, claims.UserID, card.Language)
	cs := fsrs.CardState{
		State:      fsrs.State(card.State),
		Stability:  derefFloat(card.Stability),
		Difficulty: derefFloat(card.Difficulty),
		Reps:       card.Reps,
		Lapses:     card.Lapses,
	}
	if card.LastReview != nil {
		cs.LastReview = *card.LastReview
	}

	now := time.Now().UTC()
	pv := fsrs.Preview(p, cs, now)

	writeJSON(w, http.StatusOK, map[string]any{
		"again": pv.AgainDue,
		"hard":  pv.HardDue,
		"good":  pv.GoodDue,
		"easy":  pv.EasyDue,
	})
}

// ── GET /v1/review/forecast ───────────────────────────────────────────────────

func (h *Handler) Forecast(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := r.URL.Query()
	language := q.Get("language")
	if language == "" {
		language = "ja"
	}
	days := 14
	if v := q.Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 60 {
			days = n
		}
	}

	now := time.Now().UTC()
	horizon := now.Add(time.Duration(days) * 24 * time.Hour)

	rows, err := h.db.Query(r.Context(),
		`SELECT fsrs_due FROM cards
		 WHERE user_id = $1
		   AND language_code = $2
		   AND deleted_at IS NULL
		   AND suspended = FALSE
		   AND fsrs_state IN ('review','relearning')
		   AND fsrs_due BETWEEN $3 AND $4`,
		claims.UserID, language, now, horizon,
	)
	if err != nil {
		slog.Error("forecast query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	var dues []time.Time
	for rows.Next() {
		var due time.Time
		if err := rows.Scan(&due); err == nil {
			dues = append(dues, due)
		}
	}

	counts := fsrs.WorkloadForecast(dues, days, now)

	type dayEntry struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	}
	forecast := make([]dayEntry, days)
	for i := range counts {
		forecast[i] = dayEntry{
			Date:  now.AddDate(0, 0, i).Format("2006-01-02"),
			Count: counts[i],
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"forecast":      forecast,
		"language_code": language,
	})
}

// ── GET /v1/review/notifications ─────────────────────────────────────────────

func (h *Handler) Notifications(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, type, payload, created_at FROM user_notifications
		 WHERE user_id = $1 AND read_at IS NULL
		 ORDER BY created_at DESC
		 LIMIT 20`,
		claims.UserID,
	)
	if err != nil {
		slog.Error("notifications query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	type notif struct {
		ID        string          `json:"id"`
		Type      string          `json:"type"`
		Payload   json.RawMessage `json:"payload"`
		CreatedAt time.Time       `json:"created_at"`
	}
	var notifs []notif
	for rows.Next() {
		var n notif
		if err := rows.Scan(&n.ID, &n.Type, &n.Payload, &n.CreatedAt); err == nil {
			notifs = append(notifs, n)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": notifs})
}

// ── POST /v1/review/notifications/{id}/read ───────────────────────────────────

func (h *Handler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := chi.URLParam(r, "id")
	h.db.Exec(r.Context(),
		`UPDATE user_notifications SET read_at = now()
		 WHERE id = $1 AND user_id = $2`,
		id, claims.UserID,
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── POST /v1/review/undo ──────────────────────────────────────────────────────
// Reverts the most-recent review event for the authenticated user, provided it
// was submitted within the last 10 minutes. Restores the card to its prior FSRS
// state using the snapshot stored in review_events.

func (h *Handler) Undo(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	ctx := r.Context()

	// Find the latest event within the undo window.
	var event struct {
		ID              string
		CardID          string
		PriorState      string
		PriorStability  *float64
		PriorDifficulty *float64
		PriorDue        *time.Time
		PriorReps       int
		PriorLapses     int
		ReviewedAt      time.Time
	}
	err := h.db.QueryRow(ctx,
		`SELECT id, card_id,
		        COALESCE(prior_fsrs_state, 'new'),
		        prior_fsrs_stability, prior_fsrs_difficulty, prior_fsrs_due,
		        COALESCE(prior_fsrs_reps, 0), COALESCE(prior_fsrs_lapses, 0),
		        reviewed_at
		 FROM review_events
		 WHERE user_id = $1
		   AND reviewed_at > now() - INTERVAL '10 minutes'
		 ORDER BY reviewed_at DESC
		 LIMIT 1`,
		claims.UserID,
	).Scan(
		&event.ID, &event.CardID,
		&event.PriorState, &event.PriorStability, &event.PriorDifficulty,
		&event.PriorDue, &event.PriorReps, &event.PriorLapses,
		&event.ReviewedAt,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "no recent review event to undo")
		return
	}

	// Restore card to prior state and delete the event in one transaction.
	_, err = h.db.Exec(ctx,
		`WITH deleted AS (
		     DELETE FROM review_events WHERE id = $1 AND user_id = $2
		 )
		 UPDATE cards SET
		     fsrs_state      = $3,
		     fsrs_stability  = $4,
		     fsrs_difficulty = $5,
		     fsrs_due        = $6,
		     fsrs_reps       = $7,
		     fsrs_lapses     = $8,
		     suspended       = FALSE,
		     updated_at      = now()
		 WHERE id = $9 AND user_id = $2`,
		event.ID, claims.UserID,
		event.PriorState, event.PriorStability, event.PriorDifficulty,
		event.PriorDue, event.PriorReps, event.PriorLapses,
		event.CardID,
	)
	if err != nil {
		slog.Error("review undo", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"card_id":     event.CardID,
		"undone_at":   time.Now().UTC(),
		"prior_state": event.PriorState,
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}
