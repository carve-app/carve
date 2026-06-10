package library

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
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
	// Fetch page content. The URL is user-supplied, so use the hardened client
	// that blocks internal/metadata addresses and refuses redirects (SSRF).
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	resp, err := fetchUserURL(fetchCtx, pageURL, "Carve/1.0 (+https://carve.app/bot)")
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

// ── GET /v1/library/{id}/reader ───────────────────────────────────────────────
// Returns tokenized text + unknown word list for in-browser reader mode.

func (h *Handler) Read(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := chi.URLParam(r, "id")
	ctx := r.Context()

	// Fetch item with its content.
	var pageURL, bodyText *string
	var title, language string
	err := h.db.QueryRow(ctx,
		`SELECT ci.url, ci.title, ci.language_code, ci.body_text
		 FROM user_library_items li
		 JOIN content_items ci ON ci.id = li.content_id
		 WHERE li.id = $1 AND li.user_id = $2`,
		id, claims.UserID,
	).Scan(&pageURL, &title, &language, &bodyText)
	if err != nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}

	lang := language
	if lang == "" {
		lang = "ja"
	}

	// Get text: prefer stored body_text, fall back to fetching the URL.
	var text string
	if bodyText != nil {
		text = *bodyText
	}
	if text == "" && pageURL != nil {
		fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		// User-supplied URL — use the hardened, SSRF-guarded client.
		if resp, err := fetchUserURL(fetchCtx, *pageURL, "Carve/1.0 (+https://carve.app/bot)"); err == nil {
			defer resp.Body.Close()
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 500_000))
			text = stripHTML(string(raw))
		}
	}

	if text == "" {
		writeError(w, http.StatusUnprocessableEntity, "could not extract text from item")
		return
	}

	// Truncate to 50k runes for tokenizer.
	if runes := []rune(text); len(runes) > 50_000 {
		text = string(runes[:50_000])
	}

	knownLemmas, learningLemmas := h.fetchUserVocab(ctx, claims.UserID, lang)

	// Call NLP tokenize.
	nlpBody, _ := json.Marshal(map[string]any{
		"text":            text,
		"language":        lang,
		"known_lemmas":    knownLemmas,
		"learning_lemmas": learningLemmas,
	})
	nlpCtx, nlpCancel := context.WithTimeout(ctx, 120*time.Second)
	defer nlpCancel()
	nlpReq, err := http.NewRequestWithContext(nlpCtx, http.MethodPost,
		h.nlpBaseURL+"/tokenize", bytes.NewReader(nlpBody))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	nlpReq.Header.Set("Content-Type", "application/json")
	if secret := os.Getenv("NLP_INTERNAL_SECRET"); secret != "" {
		nlpReq.Header.Set("X-Internal-Secret", secret)
	}

	nlpResp, err := http.DefaultClient.Do(nlpReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "nlp service unavailable")
		return
	}
	defer nlpResp.Body.Close()

	var nlpResult struct {
		Tokens           []json.RawMessage `json:"tokens"`
		ComprehensionPct *float64          `json:"comprehension_pct"`
	}
	if err := json.NewDecoder(nlpResp.Body).Decode(&nlpResult); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Build unknown words sidebar: content words with status=unknown, sorted by frequency_rank.
	type unknownWord struct {
		Lemma    string  `json:"lemma"`
		Reading  string  `json:"reading"`
		FreqRank *int    `json:"frequency_rank"`
	}
	seen := map[string]bool{}
	var unknown []unknownWord

	type tokenShape struct {
		Lemma         string `json:"lemma"`
		Reading       string `json:"reading_hira"`
		Status        string `json:"user_status"`
		IsContentWord bool   `json:"is_content_word"`
		FreqRank      *int   `json:"frequency_rank"`
	}
	for _, raw := range nlpResult.Tokens {
		var tok tokenShape
		if json.Unmarshal(raw, &tok) != nil {
			continue
		}
		if tok.IsContentWord && tok.Status == "unknown" && !seen[tok.Lemma] {
			seen[tok.Lemma] = true
			unknown = append(unknown, unknownWord{
				Lemma:    tok.Lemma,
				Reading:  tok.Reading,
				FreqRank: tok.FreqRank,
			})
		}
	}

	// Sort by frequency rank (lower = more common); unranked go last.
	sort.Slice(unknown, func(i, j int) bool {
		ri := unknown[i].FreqRank
		rj := unknown[j].FreqRank
		if ri == nil { return false }
		if rj == nil { return true }
		return *ri < *rj
	})
	if len(unknown) > 50 {
		unknown = unknown[:50]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":                id,
		"title":             title,
		"url":               pageURL,
		"language":          lang,
		"tokens":            nlpResult.Tokens,
		"comprehension_pct": nlpResult.ComprehensionPct,
		"unknown_words":     unknown,
	})
}

// ── POST /v1/library/import ───────────────────────────────────────────────────
// Accepts a file upload: .txt or .srt. Creates a content_item + user_library_item.

func (h *Handler) ImportFile(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	language := r.FormValue("language")
	if language == "" {
		language = "ja"
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, 5<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read file")
		return
	}

	filename := header.Filename
	ext := strings.ToLower(filename)
	var text, title string

	switch {
	case strings.HasSuffix(ext, ".txt"):
		text = string(raw)
		title = strings.TrimSuffix(filename, ".txt")
	case strings.HasSuffix(ext, ".srt"):
		text = parseSRT(string(raw))
		title = strings.TrimSuffix(filename, ".srt")
	default:
		writeError(w, http.StatusBadRequest, "unsupported file type: use .txt or .srt")
		return
	}

	if len([]rune(text)) > 500_000 {
		writeError(w, http.StatusBadRequest, "file too large (max 500 000 characters)")
		return
	}

	ctx := r.Context()
	contentID := auth.NewID()
	sourceType := "txt"
	if strings.HasSuffix(strings.ToLower(filename), ".srt") {
		sourceType = "srt"
	}

	_, err = h.db.Exec(ctx,
		`INSERT INTO content_items (id, language_code, content_type, title, body_text, source_type)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		contentID, language, sourceType, title, text, sourceType,
	)
	if err != nil {
		slog.Error("library import: insert content_item", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	itemID := auth.NewID()
	_, err = h.db.Exec(ctx,
		`INSERT INTO user_library_items (id, user_id, content_id)
		 VALUES ($1, $2, $3)`,
		itemID, claims.UserID, contentID,
	)
	if err != nil {
		slog.Error("library import: insert user_library_item", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       itemID,
		"title":    title,
		"language": language,
	})
}

// parseSRT strips timing/index lines and returns plain subtitle text.
func parseSRT(raw string) string {
	var sb strings.Builder
	lines := strings.Split(raw, "\n")
	timingRe := regexp.MustCompile(`^\d{2}:\d{2}:\d{2},\d{3} --> `)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip sequence numbers (pure integers)
		if _, err := strconv.Atoi(line); err == nil {
			continue
		}
		// Skip timing lines
		if timingRe.MatchString(line) {
			continue
		}
		// Strip inline HTML tags (e.g. <i>, <b>)
		cleaned := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(line, "")
		if cleaned != "" {
			sb.WriteString(cleaned)
			sb.WriteRune('\n')
		}
	}
	return strings.TrimSpace(sb.String())
}

// uploadMedia uploads a blob to the media service and returns its URL.
func uploadMedia(mediaBase, path string, r io.Reader, ct string) (string, error) {
	req, err := http.NewRequest(http.MethodPost, mediaBase+path, r)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", ct)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("media service returned %d", resp.StatusCode)
	}
	var res struct{ URL string `json:"url"` }
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return mediaBase + res.URL, nil
}

// multipartField reads a named field from a multipart reader (used in import).
func multipartField(mr *multipart.Reader, name string) ([]byte, *multipart.Part, error) {
	for {
		part, err := mr.NextPart()
		if err != nil {
			return nil, nil, err
		}
		if part.FormName() == name {
			data, err := io.ReadAll(io.LimitReader(part, 50<<20))
			return data, part, err
		}
		part.Close()
	}
}
