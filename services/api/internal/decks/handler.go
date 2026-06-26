package decks

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db         *pgxpool.Pool
	nlpBaseURL string
	http       *http.Client
}

func NewHandler(db *pgxpool.Pool) *Handler {
	nlp := os.Getenv("NLP_SERVICE_URL")
	if nlp == "" {
		nlp = "http://localhost:8001"
	}
	return &Handler{
		db:         db,
		nlpBaseURL: nlp,
		http:       &http.Client{Timeout: 120 * time.Second},
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

var supportedLanguages = map[string]bool{
	"ja": true, "zh-cn": true, "zh-tw": true, "ko": true, "en": true,
	"es": true, "de": true, "fr": true, "it": true, "pt": true, "vi": true,
}

func validLanguage(language string) bool { return supportedLanguages[language] }

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func unsafeText(value string) bool { return strings.ContainsRune(value, '\x00') }

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
	if !validLanguage(language) {
		writeError(w, http.StatusBadRequest, "unsupported language")
		return
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
		"decks":  decks,
		"limit":  limit,
		"offset": offset,
	})
}

// ── POST /v1/decks ────────────────────────────────────────────────────────────

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Name         string   `json:"name"`
		Description  string   `json:"description"`
		LanguageCode string   `json:"language_code"`
		Tags         []string `json:"tags"`
		IsPublic     bool     `json:"is_public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.LanguageCode == "" {
		req.LanguageCode = "ja"
	}
	if len(req.Name) > 200 || !validLanguage(req.LanguageCode) || unsafeText(req.Name) || unsafeText(req.Description) {
		writeError(w, http.StatusBadRequest, "invalid deck fields")
		return
	}
	for _, tag := range req.Tags {
		if unsafeText(tag) {
			writeError(w, http.StatusBadRequest, "invalid tag")
			return
		}
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}

	id := auth.NewID()
	ctx := r.Context()
	_, err := h.db.Exec(ctx,
		`INSERT INTO decks (id, owner_id, name, description, language_code, tags, is_public, is_official, card_count, download_count)
		 VALUES ($1, $2, $3, NULLIF($4,''), $5, $6, $7, false, 0, 0)`,
		id, claims.UserID, req.Name, req.Description, req.LanguageCode, req.Tags, req.IsPublic,
	)
	if err != nil {
		slog.Error("create deck", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": req.Name})
}

// ── PATCH /v1/decks/{id} ──────────────────────────────────────────────────────

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	deckID := chi.URLParam(r, "id")
	if !validUUID(deckID) {
		writeError(w, http.StatusBadRequest, "deck id must be a UUID")
		return
	}
	var req struct {
		Name        *string  `json:"name"`
		Description *string  `json:"description"`
		IsPublic    *bool    `json:"is_public"`
		Tags        []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if (req.Name != nil && (len(*req.Name) > 200 || unsafeText(*req.Name))) ||
		(req.Description != nil && unsafeText(*req.Description)) {
		writeError(w, http.StatusBadRequest, "invalid deck fields")
		return
	}
	for _, tag := range req.Tags {
		if unsafeText(tag) {
			writeError(w, http.StatusBadRequest, "invalid tag")
			return
		}
	}

	ctx := r.Context()
	// Verify ownership. owner_id is nullable (official/seed decks, or decks
	// whose owner was deleted → ON DELETE SET NULL), so scan into a pointer:
	// a NULL owner means the deck exists but isn't the caller's → 403, not 404.
	var ownerID *string
	if err := h.db.QueryRow(ctx, `SELECT owner_id FROM decks WHERE id = $1 AND deleted_at IS NULL`, deckID).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "deck not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if ownerID == nil || *ownerID != claims.UserID {
		writeError(w, http.StatusForbidden, "not your deck")
		return
	}

	if req.Name != nil {
		h.db.Exec(ctx, `UPDATE decks SET name = $1 WHERE id = $2`, *req.Name, deckID)
	}
	if req.Description != nil {
		h.db.Exec(ctx, `UPDATE decks SET description = NULLIF($1,'') WHERE id = $2`, *req.Description, deckID)
	}
	if req.IsPublic != nil {
		h.db.Exec(ctx, `UPDATE decks SET is_public = $1 WHERE id = $2`, *req.IsPublic, deckID)
	}
	if req.Tags != nil {
		h.db.Exec(ctx, `UPDATE decks SET tags = $1 WHERE id = $2`, req.Tags, deckID)
	}

	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

// ── DELETE /v1/decks/{id} ─────────────────────────────────────────────────────

func (h *Handler) DeleteDeck(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	deckID := chi.URLParam(r, "id")
	if !validUUID(deckID) {
		writeError(w, http.StatusBadRequest, "deck id must be a UUID")
		return
	}
	ctx := r.Context()

	var ownerID *string
	if err := h.db.QueryRow(ctx, `SELECT owner_id FROM decks WHERE id = $1 AND deleted_at IS NULL`, deckID).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "deck not found")
		} else {
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	if ownerID == nil || *ownerID != claims.UserID {
		writeError(w, http.StatusForbidden, "not your deck")
		return
	}

	h.db.Exec(ctx, `UPDATE decks SET deleted_at = NOW() WHERE id = $1`, deckID)
	w.WriteHeader(http.StatusNoContent)
}

// ── POST /v1/decks/{id}/subscribe ────────────────────────────────────────────

func (h *Handler) Subscribe(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	deckID := chi.URLParam(r, "id")
	if !validUUID(deckID) {
		writeError(w, http.StatusBadRequest, "deck id must be a UUID")
		return
	}
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
	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback(ctx)

	// Add subscription record. Track whether a NEW row was actually inserted so
	// re-subscribes / double-clicks don't inflate download_count below.
	subID := auth.NewID()
	subTag, err := tx.Exec(ctx,
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
	newSubscription := subTag.RowsAffected() == 1

	// Copy all deck cards into user's collection (new FSRS state)
	_, err = tx.Exec(ctx,
		`INSERT INTO cards
		    (id, user_id, deck_id, language_code, front_text, back_text, sentence, fsrs_state)
		 SELECT gen_random_uuid(), $1, c.deck_id, c.language_code,
		        c.front_text, c.back_text, c.sentence, 'new'
		 FROM cards c
		 JOIN decks source_deck ON source_deck.id = c.deck_id
		 WHERE c.deck_id = $2
		   AND c.deleted_at IS NULL
		   AND (
		     (source_deck.owner_id IS NOT NULL AND c.user_id = source_deck.owner_id)
		     OR (source_deck.is_official AND c.user_id = '00000000-0000-0000-0000-000000000000'::uuid)
		   )
		   AND NOT EXISTS (
		     SELECT 1 FROM cards existing
		     WHERE existing.user_id = $1
		       AND existing.deck_id = $2
		       AND existing.front_text = c.front_text
		       AND existing.deleted_at IS NULL
		   )
		 ON CONFLICT DO NOTHING`,
		claims.UserID, deckID,
	)
	if err != nil {
		slog.Error("copy deck cards", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Increment download count only for a genuinely new subscription, so the
	// public ranking metric isn't inflated by unsubscribe/re-subscribe churn.
	if newSubscription {
		if _, err := tx.Exec(ctx,
			`UPDATE decks SET download_count = download_count + 1 WHERE id = $1`,
			deckID,
		); err != nil {
			slog.Error("increment deck downloads", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("commit deck subscription", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

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
	if !validUUID(deckID) {
		writeError(w, http.StatusBadRequest, "deck id must be a UUID")
		return
	}
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
	if !validUUID(deckID) {
		writeError(w, http.StatusBadRequest, "deck id must be a UUID")
		return
	}
	var req struct {
		Rating int `json:"rating"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Rating < 1 || req.Rating > 5 {
		writeError(w, http.StatusBadRequest, "rating must be 1–5")
		return
	}

	ctx := r.Context()
	var exists bool
	if err := h.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM decks WHERE id = $1 AND deleted_at IS NULL)`,
		deckID,
	).Scan(&exists); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "deck not found")
		return
	}
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
