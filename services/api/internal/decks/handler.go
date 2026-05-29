package decks

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
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

// ── GET /v1/decks ─────────────────────────────────────────────────────────────

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
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
	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	onlyMine := q.Get("mine") == "true"

	var rows interface{ Next() bool }
	var err error

	type deckRow struct {
		ID            string    `json:"id"`
		Name          string    `json:"name"`
		Description   *string   `json:"description"`
		IsPublic      bool      `json:"is_public"`
		IsOfficial    bool      `json:"is_official"`
		Tags          []string  `json:"tags"`
		CardCount     int       `json:"card_count"`
		DownloadCount int       `json:"download_count"`
		AvgRating     *float64  `json:"avg_rating"`
		IsSubscribed  bool      `json:"is_subscribed"`
		CreatedAt     time.Time `json:"created_at"`
	}

	ctx := r.Context()
	var queryRows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
		Err() error
	}

	if onlyMine {
		queryRows, err = h.db.Query(ctx,
			`SELECT d.id, d.name, d.description, d.is_public, d.is_official,
			        COALESCE(d.tags, '{}'), d.card_count, d.download_count, d.avg_rating,
			        EXISTS(SELECT 1 FROM user_deck_subscriptions s WHERE s.deck_id = d.id AND s.user_id = $1) AS is_subscribed,
			        d.created_at
			 FROM decks d
			 WHERE d.owner_id = $1 AND d.language_code = $2 AND d.deleted_at IS NULL
			 ORDER BY d.created_at DESC
			 LIMIT $3 OFFSET $4`,
			claims.UserID, language, limit, offset,
		)
	} else {
		queryRows, err = h.db.Query(ctx,
			`SELECT d.id, d.name, d.description, d.is_public, d.is_official,
			        COALESCE(d.tags, '{}'), d.card_count, d.download_count, d.avg_rating,
			        EXISTS(SELECT 1 FROM user_deck_subscriptions s WHERE s.deck_id = d.id AND s.user_id = $1) AS is_subscribed,
			        d.created_at
			 FROM decks d
			 WHERE d.is_public = TRUE AND d.language_code = $2 AND d.deleted_at IS NULL
			 ORDER BY d.is_official DESC, d.download_count DESC, d.avg_rating DESC NULLS LAST
			 LIMIT $3 OFFSET $4`,
			claims.UserID, language, limit, offset,
		)
	}
	_ = rows
	if err != nil {
		slog.Error("list decks query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer queryRows.Close()

	var decks []deckRow
	for queryRows.Next() {
		var d deckRow
		if err := queryRows.Scan(
			&d.ID, &d.Name, &d.Description, &d.IsPublic, &d.IsOfficial,
			&d.Tags, &d.CardCount, &d.DownloadCount, &d.AvgRating,
			&d.IsSubscribed, &d.CreatedAt,
		); err != nil {
			slog.Error("scan deck row", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		decks = append(decks, d)
	}
	if err := queryRows.Err(); err != nil {
		slog.Error("deck rows error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"decks":    decks,
		"limit":    limit,
		"offset":   offset,
	})
}

// ── POST /v1/decks/{id}/subscribe ────────────────────────────────────────────

func (h *Handler) Subscribe(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	deckID := chi.URLParam(r, "id")
	ctx := r.Context()

	// Verify deck is public (or user is owner)
	var langCode string
	var isPublic bool
	err := h.db.QueryRow(ctx,
		`SELECT language_code, is_public FROM decks
		 WHERE id = $1 AND deleted_at IS NULL`,
		deckID,
	).Scan(&langCode, &isPublic)
	if err != nil {
		writeError(w, http.StatusNotFound, "deck not found")
		return
	}
	if !isPublic {
		writeError(w, http.StatusForbidden, "deck is not public")
		return
	}

	// Add subscription record
	subID := auth.NewID()
	_, err = h.db.Exec(ctx,
		`INSERT INTO user_deck_subscriptions (id, user_id, deck_id)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, deck_id) DO NOTHING`,
		subID, claims.UserID, deckID,
	)
	if err != nil {
		slog.Error("subscribe deck", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Copy all deck cards into user's collection (new FSRS state)
	_, err = h.db.Exec(ctx,
		`INSERT INTO cards
		    (id, user_id, deck_id, language_code, front_text, back_text, sentence, fsrs_state)
		 SELECT gen_random_uuid(), $1, c.deck_id, c.language_code,
		        c.front_text, c.back_text, c.sentence, 'new'
		 FROM cards c
		 WHERE c.deck_id = $2
		   AND c.deleted_at IS NULL
		   AND NOT EXISTS (
		     SELECT 1 FROM cards existing
		     WHERE existing.user_id = $1
		       AND existing.deck_id = $2
		       AND existing.front_text = c.front_text
		       AND existing.deleted_at IS NULL
		   )`,
		claims.UserID, deckID,
	)
	if err != nil {
		slog.Error("copy deck cards", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Increment download count
	h.db.Exec(ctx,
		`UPDATE decks SET download_count = download_count + 1 WHERE id = $1`,
		deckID,
	)

	writeJSON(w, http.StatusOK, map[string]any{"subscribed": true, "deck_id": deckID})
}

// ── DELETE /v1/decks/{id}/subscribe ──────────────────────────────────────────

func (h *Handler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	deckID := chi.URLParam(r, "id")
	h.db.Exec(r.Context(),
		`DELETE FROM user_deck_subscriptions WHERE user_id = $1 AND deck_id = $2`,
		claims.UserID, deckID,
	)
	w.WriteHeader(http.StatusNoContent)
}

// ── POST /v1/decks/{id}/rate ──────────────────────────────────────────────────

func (h *Handler) Rate(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	deckID := chi.URLParam(r, "id")
	var req struct {
		Rating int `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Rating < 1 || req.Rating > 5 {
		writeError(w, http.StatusBadRequest, "rating must be 1–5")
		return
	}

	ctx := r.Context()
	ratingID := auth.NewID()
	_, err := h.db.Exec(ctx,
		`INSERT INTO deck_ratings (id, deck_id, user_id, rating)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (deck_id, user_id) DO UPDATE SET rating = EXCLUDED.rating`,
		ratingID, deckID, claims.UserID, req.Rating,
	)
	if err != nil {
		slog.Error("rate deck", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Refresh avg_rating
	h.db.Exec(ctx,
		`UPDATE decks SET avg_rating = (
		   SELECT AVG(rating)::numeric(3,2) FROM deck_ratings WHERE deck_id = $1
		 ) WHERE id = $1`,
		deckID,
	)

	writeJSON(w, http.StatusOK, map[string]any{"rated": true})
}
