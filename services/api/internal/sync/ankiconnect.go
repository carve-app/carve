// Package sync implements the AnkiConnect coexist bridge. Users who
// keep Anki around for legacy reasons can sync new Carve cards into
// Anki (push) and pull review events back into Carve (pull). The bridge
// is deliberately one-way idempotent: re-running it never duplicates a
// card, and we track Anki note IDs in cards.external_anki_id so a
// second sync is a true no-op.
//
// AnkiConnect protocol reference:
//   - https://foosoft.net/projects/anki-connect/
//   - actions used here: `version`, `deckNames`, `createDeck`,
//     `addNotes`, `findCards`, `cardsInfo`
//
// The handler is deliberately not behind a /v1/sync prefix in main.go
// because it shares state with the cards package; we route it from
// settings.

package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db     *pgxpool.Pool
	client *http.Client
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db, client: &http.Client{Timeout: 15 * time.Second}}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ── AnkiConnect protocol primitives ──────────────────────────────────────────

type ankiRequest struct {
	Action  string `json:"action"`
	Version int    `json:"version"`
	Params  any    `json:"params,omitempty"`
}

type ankiResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *string         `json:"error"`
}

func (h *Handler) anki(ctx context.Context, baseURL, action string, params any) (json.RawMessage, error) {
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	req := ankiRequest{Action: action, Version: 6, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		bz, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("anki %d: %s", resp.StatusCode, string(bz))
	}
	var ar ankiResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, err
	}
	if ar.Error != nil {
		return nil, fmt.Errorf("anki: %s", *ar.Error)
	}
	return ar.Result, nil
}

// ── HTTP handlers ────────────────────────────────────────────────────────────

// POST /v1/sync/anki-connect/test
// Body: {"url": "http://localhost:8765"}
// Returns the list of Anki deck names if the bridge is reachable.
func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.ClaimsFromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct{ URL string `json:"url"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeError(w, http.StatusBadRequest, "url required")
		return
	}
	out, err := h.anki(r.Context(), req.URL, "deckNames", nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	var decks []string
	_ = json.Unmarshal(out, &decks)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "decks": decks})
}

// POST /v1/sync/anki-connect
// Body: {"url": "http://localhost:8765", "deck": "Carve Mining"}
// Pushes carve cards that don't already have an external_anki_id into the
// configured Anki deck and stores the returned note IDs. Pulls review
// events for previously-pushed cards and applies them via the FSRS path.
func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		URL  string `json:"url"`
		Deck string `json:"deck"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeError(w, http.StatusBadRequest, "url required")
		return
	}
	if req.Deck == "" {
		req.Deck = "Carve"
	}
	ctx := r.Context()

	// Ensure the target deck exists.
	if _, err := h.anki(ctx, req.URL, "createDeck", map[string]string{"deck": req.Deck}); err != nil {
		slog.Warn("anki: createDeck", "err", err)
	}

	// Find cards that haven't been pushed yet.
	rows, err := h.db.Query(ctx,
		`SELECT id, front_text, COALESCE(back_text,'') AS back_text,
		        COALESCE(sentence,'')   AS sentence
		   FROM cards
		  WHERE user_id = $1
		    AND deleted_at IS NULL
		    AND external_anki_id IS NULL
		  ORDER BY created_at ASC
		  LIMIT 500`,
		claims.UserID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query cards")
		return
	}
	defer rows.Close()

	type row struct {
		ID    string
		Front, Back, Sentence string
	}
	var pending []row
	for rows.Next() {
		var rr row
		if err := rows.Scan(&rr.ID, &rr.Front, &rr.Back, &rr.Sentence); err != nil {
			continue
		}
		pending = append(pending, rr)
	}

	pushed := 0
	if len(pending) > 0 {
		notes := make([]map[string]any, 0, len(pending))
		for _, p := range pending {
			notes = append(notes, map[string]any{
				"deckName":  req.Deck,
				"modelName": "Basic",
				"fields": map[string]string{
					"Front": p.Front,
					"Back":  p.Back + "\n\n" + p.Sentence,
				},
				"tags": []string{"carve"},
			})
		}
		out, err := h.anki(ctx, req.URL, "addNotes", map[string]any{"notes": notes})
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		var nids []*int64
		_ = json.Unmarshal(out, &nids)
		for i, nid := range nids {
			if nid == nil {
				continue
			}
			_, _ = h.db.Exec(ctx,
				`UPDATE cards SET external_anki_id = $1 WHERE id = $2`, *nid, pending[i].ID,
			)
			pushed++
		}
	}

	// Pull review events for previously-pushed cards.
	pulled, err := h.pullEvents(ctx, claims.UserID, req.URL)
	if err != nil {
		slog.Warn("anki: pullEvents", "err", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"pushed": pushed,
		"pulled": pulled,
	})
}

// pullEvents queries AnkiConnect for review events on cards we previously
// pushed and translates them into Carve review events. Returns the number
// of events applied.
func (h *Handler) pullEvents(ctx context.Context, userID, baseURL string) (int, error) {
	rows, err := h.db.Query(ctx,
		`SELECT external_anki_id, id FROM cards
		  WHERE user_id = $1 AND external_anki_id IS NOT NULL`,
		userID,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	anki2carve := map[int64]string{}
	var ids []int64
	for rows.Next() {
		var aid int64
		var cid string
		if err := rows.Scan(&aid, &cid); err == nil {
			anki2carve[aid] = cid
			ids = append(ids, aid)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}

	out, err := h.anki(ctx, baseURL, "cardsInfo", map[string]any{"cards": ids})
	if err != nil {
		return 0, err
	}
	var infos []struct {
		NoteID int64 `json:"note"`
		Reps   int   `json:"reps"`
		Lapses int   `json:"lapses"`
	}
	if err := json.Unmarshal(out, &infos); err != nil {
		return 0, err
	}

	pulled := 0
	for _, info := range infos {
		cid, ok := anki2carve[info.NoteID]
		if !ok {
			continue
		}
		// We don't have per-event timestamps from AnkiConnect's basic API,
		// so reflect the net state into cards.fsrs_reps / fsrs_lapses
		// without overwriting if we'd reduce them.
		_, _ = h.db.Exec(ctx,
			`UPDATE cards SET
			   fsrs_reps   = GREATEST(fsrs_reps,   $1),
			   fsrs_lapses = GREATEST(fsrs_lapses, $2)
			 WHERE id = $3`,
			info.Reps, info.Lapses, cid,
		)
		pulled++
	}
	return pulled, nil
}
