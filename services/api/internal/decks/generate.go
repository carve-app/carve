package decks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
)

// ── POST /v1/decks/generate ──────────────────────────────────────────────────
//
// Personalised deck generation from the user's recent library reading.
//
// Query params (or JSON body fields):
//   - language    (default "ja")
//   - since_days  (default 30, max 365)
//   - size        (default 30, max 100)
//
// Algorithm:
//  1. Pull library items added in the last `since_days` for this user+language
//     that have stored body_text (web URLs + imported text/EPUB count).
//  2. Tokenize each body via the NLP service, with the user's known+learning
//     lemmas overlaid so we only see *unknowns*.
//  3. Aggregate unknown content-word lemmas, counting encounters AND tracking
//     the lowest (best) frequency_rank seen.
//  4. Rank by encounter_count * 1/(rank+1) — the same priority signal that
//     Anki "rare-but-common-in-your-content" decks use. Take top `size`.
//  5. Batch-lookup the chosen lemmas for reading + definition.
//  6. Create a new deck owned by this user; bulk-insert cards.
//
// Returns 422 if no recent reading is found so the UI can show a clear empty
// state instead of generating a junk deck.
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Language  string `json:"language"`
		SinceDays int    `json:"since_days"`
		Size      int    `json:"size"`
	}
	// Allow query params too — easier from the web UI button.
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if v := r.URL.Query().Get("language"); v != "" {
		req.Language = v
	}
	if v := r.URL.Query().Get("since_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			req.SinceDays = n
		}
	}
	if v := r.URL.Query().Get("size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			req.Size = n
		}
	}
	if req.Language == "" {
		req.Language = "ja"
	}
	if req.SinceDays <= 0 || req.SinceDays > 365 {
		req.SinceDays = 30
	}
	if req.Size <= 0 || req.Size > 100 {
		req.Size = 30
	}

	ctx := r.Context()
	since := time.Now().UTC().AddDate(0, 0, -req.SinceDays)

	// 1) Recent library bodies for this user+language.
	bodies, err := h.fetchRecentLibraryBodies(ctx, claims.UserID, req.Language, since)
	if err != nil {
		slog.Error("generate: library scan", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(bodies) == 0 {
		writeError(w, http.StatusUnprocessableEntity,
			"no recent reading found — save a few articles in Library first")
		return
	}

	// 2-3) Per-lemma unknown aggregation.
	known, learning := h.fetchUserVocab(ctx, claims.UserID, req.Language)
	aggregates, err := h.aggregateUnknownsAcrossBodies(ctx, bodies, req.Language, known, learning)
	if err != nil {
		slog.Error("generate: tokenize aggregation", "error", err)
		writeError(w, http.StatusBadGateway, "could not tokenize bodies")
		return
	}
	if len(aggregates) == 0 {
		writeError(w, http.StatusUnprocessableEntity,
			"no new vocabulary surfaced — everything in your recent reading is already in your cards")
		return
	}

	// 4) Rank.
	ranked := rankAggregates(aggregates)
	if len(ranked) > req.Size {
		ranked = ranked[:req.Size]
	}

	// 5) Batch lookup for definitions.
	lookups := h.batchLookup(ctx, ranked, req.Language)

	// 6) Create deck + seed cards.
	deckID := auth.NewID()
	deckName := fmt.Sprintf("From your reading · %s", time.Now().UTC().Format("Jan 2"))
	deckDesc := fmt.Sprintf("Auto-generated from %d library item(s) over the last %d days",
		len(bodies), req.SinceDays)

	if _, err := h.db.Exec(ctx,
		`INSERT INTO decks (id, owner_id, name, description, language_code, tags, is_public, is_official, card_count, download_count)
		 VALUES ($1, $2, $3, $4, $5, $6, false, false, 0, 0)`,
		deckID, claims.UserID, deckName, deckDesc, req.Language,
		[]string{"personalised", "from-reading"},
	); err != nil {
		slog.Error("generate: insert deck", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	inserted := 0
	for _, a := range ranked {
		entry := lookups[a.Lemma]
		var reading, backText *string
		if entry != nil {
			if entry.Reading != "" {
				r := entry.Reading
				reading = &r
			}
			if entry.TopDefinition != "" {
				d := entry.TopDefinition
				backText = &d
			}
		}
		cardID := auth.NewID()
		if _, err := h.db.Exec(ctx,
			`INSERT INTO cards
			   (id, user_id, deck_id, language_code, front_text, front_reading, back_text, fsrs_state)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, 'new')
			 ON CONFLICT DO NOTHING`,
			cardID, claims.UserID, deckID, req.Language, a.Lemma, reading, backText,
		); err != nil {
			slog.Warn("generate: insert card", "lemma", a.Lemma, "error", err)
			continue
		}
		inserted++
	}

	// Refresh card_count.
	h.db.Exec(ctx, `UPDATE decks SET card_count = $1 WHERE id = $2`, inserted, deckID)

	writeJSON(w, http.StatusCreated, map[string]any{
		"deck_id":      deckID,
		"deck_name":    deckName,
		"card_count":   inserted,
		"sources_used": len(bodies),
	})
}

// ── helpers ──────────────────────────────────────────────────────────────────

type libraryBody struct {
	ID    string
	Title string
	Body  string
}

func (h *Handler) fetchRecentLibraryBodies(
	ctx context.Context, userID, language string, since time.Time,
) ([]libraryBody, error) {
	rows, err := h.db.Query(ctx,
		`SELECT ci.id, ci.title, ci.body_text
		   FROM user_library_items uli
		   JOIN content_items ci ON ci.id = uli.content_id
		  WHERE uli.user_id = $1
		    AND ci.language_code = $2
		    AND ci.body_text IS NOT NULL
		    AND length(ci.body_text) >= 100
		    AND uli.created_at >= $3
		  ORDER BY uli.created_at DESC
		  LIMIT 50`,
		userID, language, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []libraryBody
	for rows.Next() {
		var b libraryBody
		var body *string
		if err := rows.Scan(&b.ID, &b.Title, &body); err != nil {
			slog.Warn("generate: scan library", "error", err)
			continue
		}
		if body == nil {
			continue
		}
		// Cap individual bodies so the tokenize call stays under 50k chars
		// (matches the NLP service's text length limit).
		text := *body
		if len(text) > 45_000 {
			text = text[:45_000]
		}
		b.Body = text
		out = append(out, b)
	}
	return out, rows.Err()
}

type vocabAggregate struct {
	Lemma     string
	Encounter int
	FreqRank  int
}

func (h *Handler) aggregateUnknownsAcrossBodies(
	ctx context.Context, bodies []libraryBody, language string, known, learning []string,
) (map[string]*vocabAggregate, error) {
	out := map[string]*vocabAggregate{}
	for _, b := range bodies {
		tokens, err := h.tokenize(ctx, b.Body, language, known, learning)
		if err != nil {
			slog.Warn("generate: tokenize body", "id", b.ID, "error", err)
			continue
		}
		for _, t := range tokens {
			if !t.IsContentWord || t.UserStatus != "unknown" || t.Lemma == "" {
				continue
			}
			a, ok := out[t.Lemma]
			if !ok {
				a = &vocabAggregate{Lemma: t.Lemma, FreqRank: 0}
				out[t.Lemma] = a
			}
			a.Encounter++
			// Lowest non-zero freq_rank wins (most common form among encounters).
			if t.FrequencyRank > 0 && (a.FreqRank == 0 || t.FrequencyRank < a.FreqRank) {
				a.FreqRank = t.FrequencyRank
			}
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// rankAggregates orders unknowns by encounter_count * 1/(freq_rank+1) — words
// the user saw often AND that are common in the corpus rise to the top. Words
// with no freq_rank fall back to pure encounter ranking.
func rankAggregates(m map[string]*vocabAggregate) []*vocabAggregate {
	out := make([]*vocabAggregate, 0, len(m))
	for _, a := range m {
		out = append(out, a)
	}
	score := func(a *vocabAggregate) float64 {
		r := a.FreqRank
		if r <= 0 {
			r = 50_000
		}
		return float64(a.Encounter) * (1.0 / float64(r+1))
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := score(out[i]), score(out[j])
		if si != sj {
			return si > sj
		}
		// Tie-break: higher encounter count wins, then lower freq_rank.
		if out[i].Encounter != out[j].Encounter {
			return out[i].Encounter > out[j].Encounter
		}
		return out[i].FreqRank < out[j].FreqRank
	})
	return out
}

// ── NLP service calls ────────────────────────────────────────────────────────

type nlpToken struct {
	Lemma         string `json:"lemma"`
	Surface       string `json:"surface"`
	Reading       string `json:"reading"`
	ReadingHira   string `json:"reading_hira"`
	IsContentWord bool   `json:"is_content_word"`
	UserStatus    string `json:"user_status"`
	FrequencyRank int    `json:"frequency_rank"`
}

func (h *Handler) tokenize(
	ctx context.Context, text, language string, known, learning []string,
) ([]nlpToken, error) {
	if text == "" {
		return nil, nil
	}
	body, _ := json.Marshal(map[string]any{
		"text":            text,
		"language":        language,
		"known_lemmas":    known,
		"learning_lemmas": learning,
	})
	ctxT, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctxT, http.MethodPost, h.nlpBaseURL+"/tokenize", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret := os.Getenv("NLP_INTERNAL_SECRET"); secret != "" {
		req.Header.Set("X-Internal-Secret", secret)
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("tokenize: non-200")
	}
	var result struct {
		Tokens []nlpToken `json:"tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Tokens, nil
}

type lookupEntry struct {
	Reading       string
	TopDefinition string
}

func (h *Handler) batchLookup(ctx context.Context, ranked []*vocabAggregate, language string) map[string]*lookupEntry {
	out := map[string]*lookupEntry{}
	if language != "ja" || len(ranked) == 0 {
		return out
	}
	lemmas := make([]string, 0, len(ranked))
	for _, a := range ranked {
		lemmas = append(lemmas, a.Lemma)
	}
	body, _ := json.Marshal(map[string]any{
		"lemmas":   lemmas,
		"language": language,
	})
	ctxL, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctxL, http.MethodPost, h.nlpBaseURL+"/batch-lookup", bytes.NewReader(body))
	if err != nil {
		return out
	}
	req.Header.Set("Content-Type", "application/json")
	if secret := os.Getenv("NLP_INTERNAL_SECRET"); secret != "" {
		req.Header.Set("X-Internal-Secret", secret)
	}
	resp, err := h.http.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return out
	}
	defer resp.Body.Close()
	var result struct {
		Results map[string]*struct {
			Reading     string `json:"reading"`
			Definitions []struct {
				Definition string `json:"definition"`
			} `json:"definitions"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return out
	}
	for lemma, e := range result.Results {
		if e == nil {
			continue
		}
		entry := &lookupEntry{Reading: e.Reading}
		if len(e.Definitions) > 0 {
			entry.TopDefinition = e.Definitions[0].Definition
		}
		out[lemma] = entry
	}
	return out
}

func (h *Handler) fetchUserVocab(ctx context.Context, userID, language string) (known, learning []string) {
	rows, err := h.db.Query(ctx,
		`SELECT front_text, fsrs_state
		   FROM cards
		  WHERE user_id = $1 AND language_code = $2 AND deleted_at IS NULL`,
		userID, language,
	)
	if err != nil {
		return nil, nil
	}
	defer rows.Close()
	for rows.Next() {
		var lemma, state string
		if rows.Scan(&lemma, &state) == nil {
			switch state {
			case "review":
				known = append(known, lemma)
			case "learning", "relearning":
				learning = append(learning, lemma)
			default:
				// any card the user has counts as "seen" — exclude from generated deck
				known = append(known, lemma)
			}
		}
	}
	return known, learning
}
