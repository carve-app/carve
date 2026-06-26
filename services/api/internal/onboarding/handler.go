package onboarding

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

// POST /v1/onboarding/known-words
// Seeds user_word_knowledge for words the user already knows.
func (h *Handler) KnownWords(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Language string   `json:"language"`
		Lemmas   []string `json:"lemmas"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Language == "" || len(req.Lemmas) == 0 {
		writeError(w, http.StatusBadRequest, "language and lemmas are required")
		return
	}

	ctx := r.Context()
	marked := 0

	for _, lemma := range req.Lemmas {
		var wordID string
		err := h.db.QueryRow(ctx,
			`INSERT INTO words (language_code, lemma, reading)
			 VALUES ($1, $2, $2)
			 ON CONFLICT (language_code, lemma, reading) DO UPDATE SET language_code = EXCLUDED.language_code
			 RETURNING id`,
			req.Language, lemma,
		).Scan(&wordID)
		if err != nil {
			continue
		}

		_, err = h.db.Exec(ctx,
			`INSERT INTO user_word_knowledge (user_id, word_id, status, known_since)
			 VALUES ($1, $2, 'known', now())
			 ON CONFLICT (user_id, word_id) DO UPDATE
			   SET status = 'known', known_since = COALESCE(user_word_knowledge.known_since, now())`,
			claims.UserID, wordID,
		)
		if err == nil {
			marked++
		}
	}

	writeJSON(w, http.StatusOK, map[string]int{"marked": marked})
}

// POST /v1/onboarding/starter-deck
// Subscribes the user to the official starter deck for their chosen language.
func (h *Handler) StarterDeck(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Language == "" {
		writeError(w, http.StatusBadRequest, "language is required")
		return
	}

	ctx := r.Context()

	var deckID string
	err := h.db.QueryRow(ctx,
		`SELECT id FROM decks
		 WHERE language_code = $1 AND is_official = TRUE AND deleted_at IS NULL
		 ORDER BY name LIMIT 1`,
		req.Language,
	).Scan(&deckID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "no_deck"})
		return
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_deck_subscriptions (user_id, deck_id)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id, deck_id) DO NOTHING`,
		claims.UserID, deckID,
	); err != nil {
		slog.Error("subscribe starter deck", "error", err, "user_id", claims.UserID, "deck_id", deckID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO cards
		    (id, user_id, deck_id, language_code, front_text, back_text, sentence, fsrs_state)
		 SELECT gen_random_uuid(), $1, c.deck_id, c.language_code,
		        c.front_text, c.back_text, c.sentence, 'new'
		 FROM cards c
		 WHERE c.deck_id = $2
		   AND c.user_id = '00000000-0000-0000-0000-000000000000'::uuid
		   AND c.deleted_at IS NULL
		   AND NOT EXISTS (
		     SELECT 1 FROM cards existing
		     WHERE existing.user_id = $1
		       AND existing.deck_id = $2
		       AND existing.front_text = c.front_text
		       AND existing.deleted_at IS NULL
		   )
		 ON CONFLICT DO NOTHING`,
		claims.UserID, deckID,
	); err != nil {
		slog.Error("copy starter deck", "error", err, "user_id", claims.UserID, "deck_id", deckID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("commit starter deck", "error", err, "user_id", claims.UserID, "deck_id", deckID)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"deck_id": deckID, "status": "subscribed"})
}
