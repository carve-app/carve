package onboarding

import (
	"encoding/json"
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

	h.db.Exec(ctx,
		`INSERT INTO user_deck_subscriptions (user_id, deck_id)
		 VALUES ($1, $2)
		 ON CONFLICT (user_id, deck_id) DO NOTHING`,
		claims.UserID, deckID,
	)

	writeJSON(w, http.StatusOK, map[string]string{"deck_id": deckID, "status": "subscribed"})
}
