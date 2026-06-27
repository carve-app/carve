package review

import (
	"context"
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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func reviewLanguage(r *http.Request) (string, bool) {
	language := r.URL.Query().Get("language")
	if language == "" {
		return "ja", true
	}
	switch language {
	case "ja", "zh-cn", "zh-tw", "ko", "en", "es", "de", "fr", "it", "pt", "vi":
		return language, true
	default:
		return "", false
	}
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
	var leechThreshold int
	err := h.db.QueryRow(r.Context(),
		`SELECT weights, target_retention, leech_threshold FROM user_fsrs_params
		 WHERE user_id = $1 AND language_code = $2`,
		userID, lang,
	).Scan(&weightsJSON, &retention, &leechThreshold)
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
	if leechThreshold > 0 {
		p.LeechThreshold = leechThreshold
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

	language, languageOK := reviewLanguage(r)
	if !languageOK {
		writeError(w, http.StatusBadRequest, "unsupported language")
		return
	}
	_, _ = h.db.Exec(r.Context(),
		`UPDATE cards SET buried = FALSE, buried_until = NULL
		 WHERE user_id = $1 AND buried = TRUE AND buried_until <= CURRENT_DATE`,
		claims.UserID,
	)

	dailyNewLimit := h.dailyNewRemaining(r.Context(), claims.UserID, language)
	var scheduledCount, newCount int
	err := h.db.QueryRow(r.Context(),
		`SELECT
		   COUNT(*) FILTER (WHERE fsrs_state <> 'new' AND fsrs_due <= now()),
		   COUNT(*) FILTER (WHERE fsrs_state = 'new')
		 FROM cards
		 WHERE user_id = $1
		   AND language_code = $2
		   AND deleted_at IS NULL
		   AND suspended = FALSE
		   AND buried = FALSE`,
		claims.UserID, language,
	).Scan(&scheduledCount, &newCount)
	if err != nil {
		slog.Error("due count query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if newCount > dailyNewLimit {
		newCount = dailyNewLimit
	}
	count := scheduledCount + newCount

	writeJSON(w, http.StatusOK, map[string]any{
		"due_count":     count,
		"language_code": language,
	})
}

// ── GET /v1/review/session ────────────────────────────────────────────────────

type sessionCard struct {
	ID            string     `json:"id"`
	FrontText     string     `json:"front_text"`
	CardType      string     `json:"card_type"`
	BackText      *string    `json:"back_text"`
	Sentence      *string    `json:"sentence"`
	Translation   *string    `json:"subtitle_translation"`
	SourceURL     *string    `json:"source_url"`
	AudioURL      *string    `json:"audio_url"`
	SentenceAudio *string    `json:"sentence_audio_url"`
	ImageURL      *string    `json:"image_url"`
	FsrsState     string     `json:"fsrs_state"`
	Stability     *float64   `json:"stability"`
	Difficulty    *float64   `json:"difficulty"`
	Due           *time.Time `json:"due"`
	Reps          int        `json:"reps"`
	Lapses        int        `json:"lapses"`
}

func (h *Handler) Session(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := r.URL.Query()
	language, languageOK := reviewLanguage(r)
	if !languageOK {
		writeError(w, http.StatusBadRequest, "unsupported language")
		return
	}
	_, _ = h.db.Exec(r.Context(),
		`UPDATE cards SET buried = FALSE, buried_until = NULL
		 WHERE user_id = $1 AND buried = TRUE AND buried_until <= CURRENT_DATE`,
		claims.UserID,
	)
	limit := 20
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	// Fetch all due cards, learning-priority first (smallest due time first)
	rows, err := h.db.Query(r.Context(),
		`SELECT id, front_text, card_type, back_text, sentence, subtitle_translation, source_url,
		        front_audio_url, sentence_audio_url, front_image_url,
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
			&c.ID, &c.FrontText, &c.CardType, &c.BackText, &c.Sentence, &c.Translation, &c.SourceURL,
			&c.AudioURL, &c.SentenceAudio, &c.ImageURL,
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

	// Desimilarizer: prefer at most 2 cards per first-rune bucket to avoid
	// confusingly similar cards (e.g., 食べる / 食べない) clustering in the same
	// session. The bucket cap only spreads cards out — it must never discard
	// due work, so if the first pass falls short of `limit` we backfill with
	// the remaining (priority-ordered) cards, ignoring the cap.
	const maxPerBucket = 2
	seen := make(map[string]int)
	selected := make([]bool, len(all))
	var session []sessionCard
	newRemaining := h.dailyNewRemaining(r.Context(), claims.UserID, language)
	newSelected := 0
	eligible := func(c rawCard) bool {
		return c.FsrsState != "new" || newSelected < newRemaining
	}
	for i, c := range all {
		if len(session) >= limit {
			break
		}
		if eligible(c) && seen[c.sortKey] < maxPerBucket {
			session = append(session, c.sessionCard)
			seen[c.sortKey]++
			selected[i] = true
			if c.FsrsState == "new" {
				newSelected++
			}
		}
	}
	// Backfill: append not-yet-selected cards (in DB priority order) until we
	// reach the limit, so a deck whose due cards share leading characters still
	// serves a full session.
	for i, c := range all {
		if len(session) >= limit {
			break
		}
		if !selected[i] && eligible(c) {
			session = append(session, c.sessionCard)
			selected[i] = true
			if c.FsrsState == "new" {
				newSelected++
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"cards":         session,
		"total":         len(session),
		"language_code": language,
	})
}

func (h *Handler) dailyNewRemaining(ctx context.Context, userID, language string) int {
	limit := 20
	_ = h.db.QueryRow(ctx,
		`SELECT daily_new_limit FROM user_fsrs_params
		 WHERE user_id = $1 AND language_code = $2`,
		userID, language,
	).Scan(&limit)
	var reviewedToday int
	_ = h.db.QueryRow(ctx,
		`SELECT COUNT(*)
		 FROM review_events e
		 JOIN cards c ON c.id = e.card_id
		 WHERE e.user_id = $1
		   AND c.language_code = $2
		   AND e.prior_fsrs_state = 'new'
		   AND e.reviewed_at >= CURRENT_DATE`,
		userID, language,
	).Scan(&reviewedToday)
	if remaining := limit - reviewedToday; remaining > 0 {
		return remaining
	}
	return 0
}

// ── POST /v1/review/events ────────────────────────────────────────────────────

type eventResponse struct {
	State      string    `json:"state"`
	Stability  float64   `json:"stability"`
	Difficulty float64   `json:"difficulty"`
	Due        time.Time `json:"due"`
	Reps       int       `json:"reps"`
	Lapses     int       `json:"lapses"`
	IsLeech    bool      `json:"is_leech"`
}

func existingEvent(ctx context.Context, tx pgx.Tx, userID, clientEventID string) (eventResponse, bool, error) {
	var out eventResponse
	var responseJSON []byte
	err := tx.QueryRow(ctx,
		`SELECT response_after
		 FROM review_events
		 WHERE user_id = $1 AND client_event_id = $2`,
		userID, clientEventID,
	).Scan(&responseJSON)
	if err == pgx.ErrNoRows {
		return eventResponse{}, false, nil
	}
	if err != nil {
		return eventResponse{}, false, err
	}
	if err := json.Unmarshal(responseJSON, &out); err != nil {
		return eventResponse{}, false, err
	}
	return out, true, nil
}

func (h *Handler) SubmitEvent(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		EventID    string `json:"event_id"`
		CardID     string `json:"card_id"`
		Rating     int    `json:"rating"`
		TimeMS     *int   `json:"time_taken_ms"`
		ReviewedAt string `json:"reviewed_at"` // ISO 8601; empty = now
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
	if req.EventID != "" {
		if _, err := uuid.Parse(req.EventID); err != nil {
			writeError(w, http.StatusBadRequest, "event_id must be a UUID")
			return
		}
	}

	reviewedAt := time.Now().UTC()
	if req.ReviewedAt != "" {
		if t, err := time.Parse(time.RFC3339, req.ReviewedAt); err == nil {
			reviewedAt = t.UTC()
		}
	}

	ctx := r.Context()

	// The card read, FSRS recompute, card update, and review-event insert must
	// be atomic: two concurrent submissions for the same card (extension + web
	// tab, double-tap, retry) would otherwise both read the same prior state
	// and the later write would clobber the earlier — losing a rep/lapse and
	// storing a duplicate prior_* snapshot that corrupts Undo. SELECT ... FOR
	// UPDATE serializes them on the card row.
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback(ctx)

	// Fast replay path. This runs before card validation so a retry of the event
	// that suspended a leech still returns its original success response.
	if req.EventID != "" {
		if prior, found, err := existingEvent(ctx, tx, claims.UserID, req.EventID); err != nil {
			slog.Error("lookup review event replay", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		} else if found {
			writeJSON(w, http.StatusOK, prior)
			return
		}
	}

	var card struct {
		Stability  *float64
		Difficulty *float64
		Due        *time.Time
		LastReview *time.Time
		State      string
		Reps       int
		Lapses     int
		Language   string
		Suspended  bool
		IsLeech    bool
	}
	err = tx.QueryRow(ctx,
		`SELECT fsrs_stability, fsrs_difficulty, fsrs_due, fsrs_last_review,
		        fsrs_state, fsrs_reps, fsrs_lapses, language_code,
		        suspended, is_leech
		 FROM cards
		 WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL AND suspended = FALSE
		 FOR UPDATE`,
		req.CardID, claims.UserID,
	).Scan(
		&card.Stability, &card.Difficulty, &card.Due, &card.LastReview,
		&card.State, &card.Reps, &card.Lapses, &card.Language,
		&card.Suspended, &card.IsLeech,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}

	// A concurrent request with the same client ID may have committed while we
	// waited for the card row lock. Re-check after the lock before scheduling.
	if req.EventID != "" {
		if prior, found, err := existingEvent(ctx, tx, claims.UserID, req.EventID); err != nil {
			slog.Error("lookup concurrent review replay", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		} else if found {
			writeJSON(w, http.StatusOK, prior)
			return
		}
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
	response := eventResponse{
		State: string(result.State), Stability: result.Stability,
		Difficulty: result.Difficulty, Due: result.Due, Reps: result.Reps,
		Lapses: result.Lapses, IsLeech: result.IsLeech,
	}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Persist updated card state
	_, err = tx.Exec(ctx,
		`UPDATE cards SET
		   fsrs_state      = $1,
		   fsrs_stability  = $2,
		   fsrs_difficulty = $3,
		   fsrs_due        = $4,
		   fsrs_last_review= $5,
		   fsrs_reps       = $6,
		   fsrs_lapses     = $7,
		   suspended       = $8,
		   is_leech        = is_leech OR $8
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

	// Store review event with prior state snapshot (needed for undo). Inside the
	// same tx so its failure rolls back the card update — state and history stay
	// consistent.
	eventID := auth.NewID()
	var clientEventID any
	if req.EventID != "" {
		clientEventID = req.EventID
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO review_events
		   (id, client_event_id, card_id, user_id, reviewed_at, rating, time_taken_ms,
		    stability_after, difficulty_after, due_after, retrievability_at_review,
		    state_after, reps_after, lapses_after, is_leech_after, response_after,
		    prior_fsrs_state, prior_fsrs_stability, prior_fsrs_difficulty,
		    prior_fsrs_due, prior_fsrs_last_review, prior_fsrs_reps, prior_fsrs_lapses,
		    prior_suspended, prior_is_leech)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)`,
		eventID, clientEventID, req.CardID, claims.UserID, reviewedAt, req.Rating, req.TimeMS,
		result.Stability, result.Difficulty, result.Due, result.Retrievability,
		string(result.State), result.Reps, result.Lapses, result.IsLeech, responseJSON,
		card.State, card.Stability, card.Difficulty, card.Due, card.LastReview, card.Reps, card.Lapses,
		card.Suspended, card.IsLeech,
	)
	if err != nil {
		slog.Error("insert review event", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		slog.Error("commit review event", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	metrics.IncCounter("fsrs_event_total")

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

	writeJSON(w, http.StatusOK, response)
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
	language, languageOK := reviewLanguage(r)
	if !languageOK {
		writeError(w, http.StatusBadRequest, "unsupported language")
		return
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
	notifs := []notif{}
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
		PriorLastReview *time.Time
		PriorReps       int
		PriorLapses     int
		PriorSuspended  bool
		PriorIsLeech    bool
		ReviewedAt      time.Time
	}
	err := h.db.QueryRow(ctx,
		`SELECT id, card_id,
		        COALESCE(prior_fsrs_state, 'new'),
		        prior_fsrs_stability, prior_fsrs_difficulty, prior_fsrs_due,
		        prior_fsrs_last_review,
		        COALESCE(prior_fsrs_reps, 0), COALESCE(prior_fsrs_lapses, 0),
		        COALESCE(prior_suspended, FALSE), COALESCE(prior_is_leech, FALSE),
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
		&event.PriorDue, &event.PriorLastReview, &event.PriorReps, &event.PriorLapses,
		&event.PriorSuspended, &event.PriorIsLeech,
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
		 ), deleted_notification AS (
		     DELETE FROM user_notifications
		     WHERE user_id = $2
		       AND type = 'leech_suspended'
		       AND payload->>'card_id' = $13
		       AND created_at >= $14
		 )
		 UPDATE cards SET
		     fsrs_state      = $3,
		     fsrs_stability  = $4,
		     fsrs_difficulty = $5,
		     fsrs_due        = $6,
		     fsrs_last_review= $7,
		     fsrs_reps       = $8,
		     fsrs_lapses     = $9,
		     suspended       = $10,
		     is_leech        = $11,
		     updated_at      = now()
		 WHERE id = $12 AND user_id = $2`,
		event.ID, claims.UserID,
		event.PriorState, event.PriorStability, event.PriorDifficulty,
		event.PriorDue, event.PriorLastReview, event.PriorReps, event.PriorLapses,
		event.PriorSuspended, event.PriorIsLeech,
		event.CardID, event.CardID, event.ReviewedAt,
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
