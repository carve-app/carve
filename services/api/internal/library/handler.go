package library

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db         *pgxpool.Pool
	nlpBaseURL string
}

func NewHandler(db *pgxpool.Pool) *Handler {
	nlp := os.Getenv("NLP_SERVICE_URL")
	if nlp == "" {
		nlp = "http://localhost:8001"
	}
	return &Handler{db: db, nlpBaseURL: nlp}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ── GET /v1/library ───────────────────────────────────────────────────────────

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	language := r.URL.Query().Get("language")
	if language == "" {
		language = "ja"
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT
		    li.id, ci.url, ci.title, li.comprehension_pct, li.unknown_word_count,
		    li.created_at, ci.content_type
		 FROM user_library_items li
		 JOIN content_items ci ON ci.id = li.content_id
		 WHERE li.user_id = $1 AND ci.language_code = $2
		 ORDER BY li.created_at DESC
		 LIMIT 100`,
		claims.UserID, language,
	)
	if err != nil {
		slog.Error("library list query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	type item struct {
		ID               string     `json:"id"`
		URL              *string    `json:"url"`
		Title            string     `json:"title"`
		ComprehensionPct *float64   `json:"comprehension_pct"`
		UnknownWordCount *int       `json:"unknown_word_count"`
		ContentType      string     `json:"content_type"`
		CreatedAt        time.Time  `json:"created_at"`
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(
			&it.ID, &it.URL, &it.Title, &it.ComprehensionPct,
			&it.UnknownWordCount, &it.CreatedAt, &it.ContentType,
		); err != nil {
			slog.Error("library list scan", "error", err)
			continue
		}
		items = append(items, it)
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ── POST /v1/library ──────────────────────────────────────────────────────────
// Body: {"url": "...", "language": "ja"}

func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		URL      string `json:"url"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url required")
		return
	}
	if req.Language == "" {
		req.Language = "ja"
	}

	// Validate URL format.
	parsed, err := url.ParseRequestURI(req.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeError(w, http.StatusBadRequest, "invalid url: must be http or https")
		return
	}

	ctx := r.Context()

	// Fetch user vocab for comprehension scoring.
	knownLemmas, learningLemmas := h.fetchUserVocab(ctx, claims.UserID, req.Language)

	// Fetch and score the URL.
	score, title, err := h.scoreURL(ctx, req.URL, req.Language, knownLemmas, learningLemmas)
	if err != nil {
		slog.Warn("library: score url failed", "url", req.URL, "error", err)
		// Continue with nil score — still save the URL.
	}

	// Upsert content_items row.
	contentID := auth.NewID()
	displayTitle := title
	if displayTitle == "" {
		displayTitle = req.URL
	}
	_, err = h.db.Exec(ctx,
		`INSERT INTO content_items (id, language_code, content_type, url, title)
		 VALUES ($1, $2, 'article', $3, $4)
		 ON CONFLICT DO NOTHING`,
		contentID, req.Language, req.URL, displayTitle,
	)
	if err != nil {
		slog.Error("library: insert content_item", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Fetch actual content_id (may have been pre-existing).
	var actualContentID string
	if err := h.db.QueryRow(ctx,
		`SELECT id FROM content_items WHERE url = $1 AND language_code = $2`,
		req.URL, req.Language,
	).Scan(&actualContentID); err != nil {
		actualContentID = contentID
	}

	// Upsert user_library_items.
	itemID := auth.NewID()
	var compPct *float64
	var unknownCount *int
	if score != nil {
		compPct = &score.ComprehensionPct
		unknownCount = &score.UnknownCount
	}

	_, err = h.db.Exec(ctx,
		`INSERT INTO user_library_items
		    (id, user_id, content_id, comprehension_pct, unknown_word_count)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (user_id, content_id) DO UPDATE SET
		   comprehension_pct = EXCLUDED.comprehension_pct,
		   unknown_word_count = EXCLUDED.unknown_word_count,
		   updated_at = now()`,
		itemID, claims.UserID, actualContentID, compPct, unknownCount,
	)
	if err != nil {
		slog.Error("library: insert user_library_item", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":                itemID,
		"url":               req.URL,
		"title":             displayTitle,
		"comprehension_pct": compPct,
		"unknown_word_count": unknownCount,
	})
}

// ── DELETE /v1/library/{id} ───────────────────────────────────────────────────

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := chi.URLParam(r, "id")
	tag, err := h.db.Exec(r.Context(),
		`DELETE FROM user_library_items WHERE id = $1 AND user_id = $2`,
		id, claims.UserID,
	)
	if err != nil {
		slog.Error("library delete", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ───────────────────────────────────────────────────────────────────

type scoreResult struct {
	ComprehensionPct float64
	UnknownCount     int
}

func (h *Handler) scoreURL(
	ctx context.Context,
	pageURL, language string,
	knownLemmas, learningLemmas []string,
) (*scoreResult, string, error) {
	// Fetch page content.
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Carve/1.0 (+https://carve.app/bot)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	rawBytes, err := io.ReadAll(io.LimitReader(resp.Body, 500_000))
	if err != nil {
		return nil, "", err
	}

	title := extractTitle(string(rawBytes))
	text := stripHTML(string(rawBytes))
	if len([]rune(text)) > 50_000 {
		runes := []rune(text)
		text = string(runes[:50_000])
	}
	if text == "" {
		return nil, title, nil
	}

	// Call NLP score-text endpoint.
	body, _ := json.Marshal(map[string]any{
		"text":            text,
		"language":        language,
		"known_lemmas":    knownLemmas,
		"learning_lemmas": learningLemmas,
	})
	nlpCtx, nlpCancel := context.WithTimeout(ctx, 30*time.Second)
	defer nlpCancel()
	nlpReq, err := http.NewRequestWithContext(nlpCtx, http.MethodPost,
		h.nlpBaseURL+"/score-text", bytes.NewReader(body))
	if err != nil {
		return nil, title, err
	}
	nlpReq.Header.Set("Content-Type", "application/json")
	if secret := os.Getenv("NLP_INTERNAL_SECRET"); secret != "" {
		nlpReq.Header.Set("X-Internal-Secret", secret)
	}

	nlpResp, err := http.DefaultClient.Do(nlpReq)
	if err != nil {
		return nil, title, err
	}
	defer nlpResp.Body.Close()

	var nlpResult struct {
		ComprehensionPct float64 `json:"comprehension_pct"`
		UnknownCount     int     `json:"unknown_count"`
	}
	if err := json.NewDecoder(nlpResp.Body).Decode(&nlpResult); err != nil {
		return nil, title, err
	}

	return &scoreResult{
		ComprehensionPct: nlpResult.ComprehensionPct,
		UnknownCount:     nlpResult.UnknownCount,
	}, title, nil
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
			}
		}
	}
	return known, learning
}

// extractTitle pulls the first <title> content from HTML.
func extractTitle(html string) string {
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<title>")
	if start < 0 {
		return ""
	}
	start += len("<title>")
	end := strings.Index(lower[start:], "</title>")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(html[start : start+end])
}

// stripHTML removes HTML tags and returns plain text.
func stripHTML(html string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
			sb.WriteRune(' ')
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	// Collapse whitespace.
	return strings.Join(strings.Fields(sb.String()), " ")
}
