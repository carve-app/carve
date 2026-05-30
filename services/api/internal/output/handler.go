// Package output provides the output practice API: writing drills, cloze,
// shadowing queue, and AI feedback via the Claude API.
package output

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db         *pgxpool.Pool
	claudeKey  string
	claudeURL  string
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{
		db:        db,
		claudeKey: os.Getenv("ANTHROPIC_API_KEY"),
		claudeURL: "https://api.anthropic.com/v1/messages",
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

// ── GET /v1/output/exercises ──────────────────────────────────────────────────
// Returns a mix of writing, cloze, and shadowing exercises generated from the
// user's recently mined cards.

func (h *Handler) ListExercises(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	lang := r.URL.Query().Get("language")
	if lang == "" {
		lang = "ja"
	}

	ctx := r.Context()

	// Fetch recent cards with sentences as exercise material
	type cardRow struct {
		ID          string
		FrontText   string
		Sentence    string
		AudioURL    *string
		Translation *string
	}

	rows, err := h.db.Query(ctx,
		`SELECT id, front_text, COALESCE(sentence,''), front_audio_url, translation
		 FROM cards
		 WHERE user_id = $1
		   AND language_code = $2
		   AND deleted_at IS NULL
		   AND fsrs_state IN ('review','relearning','learning')
		   AND sentence IS NOT NULL AND sentence != ''
		 ORDER BY updated_at DESC
		 LIMIT 20`,
		claims.UserID, lang,
	)
	if err != nil {
		slog.Error("list output exercises", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	var cards []cardRow
	for rows.Next() {
		var c cardRow
		if err := rows.Scan(&c.ID, &c.FrontText, &c.Sentence, &c.AudioURL, &c.Translation); err != nil {
			continue
		}
		cards = append(cards, c)
	}

	// Generate exercise instances (upsert then return)
	type Exercise struct {
		ID           string  `json:"id"`
		ExerciseType string  `json:"exercise_type"`
		Prompt       string  `json:"prompt"`
		TargetWord   string  `json:"target_word"`
		Sentence     *string `json:"sentence,omitempty"`
		AudioURL     *string `json:"audio_url,omitempty"`
		CreatedAt    string  `json:"created_at"`
	}

	var exercises []Exercise
	now := time.Now().UTC()

	for _, card := range cards {
		exType, prompt := chooseExerciseType(card.FrontText, card.Sentence, card.AudioURL)

		// Upsert exercise
		exID := auth.NewID()
		var sentence *string
		if card.Sentence != "" {
			s := card.Sentence
			sentence = &s
		}
		_, err := h.db.Exec(ctx,
			`INSERT INTO output_exercises
			    (id, user_id, card_id, language_code, exercise_type, prompt, target_word, sentence, audio_url)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT DO NOTHING`,
			exID, claims.UserID, card.ID, lang, exType, prompt, card.FrontText, sentence, card.AudioURL,
		)
		if err != nil {
			// Already exists or error — skip
			continue
		}

		exercises = append(exercises, Exercise{
			ID:           exID,
			ExerciseType: exType,
			Prompt:       prompt,
			TargetWord:   card.FrontText,
			Sentence:     sentence,
			AudioURL:     card.AudioURL,
			CreatedAt:    now.Format(time.RFC3339),
		})

		if len(exercises) >= 10 {
			break
		}
	}

	if exercises == nil {
		exercises = []Exercise{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"exercises": exercises})
}

func chooseExerciseType(word, sentence string, audioURL *string) (string, string) {
	switch {
	case audioURL != nil && *audioURL != "":
		return "shadowing", fmt.Sprintf("Listen and repeat: %s", word)
	case sentence != "":
		// Make a cloze by blanking the target word
		blanked := strings.ReplaceAll(sentence, word, "___")
		return "cloze", fmt.Sprintf("Fill in the blank: %s", blanked)
	default:
		return "writing", fmt.Sprintf("Write a sentence using the word: %s", word)
	}
}

// ── POST /v1/output/submit ────────────────────────────────────────────────────
// Submit an exercise answer and receive AI feedback.

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		ExerciseID string `json:"exercise_id"`
		AnswerText string `json:"answer_text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ExerciseID == "" {
		writeError(w, http.StatusBadRequest, "exercise_id and answer_text required")
		return
	}

	ctx := r.Context()

	// Load exercise details
	var exType, prompt, targetWord, lang string
	var sentence *string
	err := h.db.QueryRow(ctx,
		`SELECT exercise_type, prompt, target_word, language_code, sentence
		 FROM output_exercises
		 WHERE id = $1 AND user_id = $2 AND expires_at > now()`,
		req.ExerciseID, claims.UserID,
	).Scan(&exType, &prompt, &targetWord, &lang, &sentence)
	if err != nil {
		writeError(w, http.StatusNotFound, "exercise not found or expired")
		return
	}

	// Get AI feedback
	feedback, score, detail, err := h.getFeedback(ctx, req.AnswerText, targetWord, lang, exType, sentence)
	if err != nil {
		slog.Error("get feedback", "error", err)
		// Return a graceful fallback rather than 500
		feedback = "Unable to get AI feedback right now. Keep practicing!"
		score = 0
	}

	// Store submission
	subID := auth.NewID()
	detailJSON, _ := json.Marshal(detail)
	h.db.Exec(ctx,
		`INSERT INTO output_submissions
		    (id, exercise_id, user_id, answer_text, score, feedback, feedback_detail)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		subID, req.ExerciseID, claims.UserID, req.AnswerText, score, feedback, detailJSON,
	)

	writeJSON(w, http.StatusOK, map[string]any{
		"submission_id":   subID,
		"score":           score,
		"feedback":        feedback,
		"feedback_detail": detail,
	})
}

type FeedbackDetail struct {
	Grammar      string `json:"grammar"`
	Vocabulary   string `json:"vocabulary"`
	Naturalness  string `json:"naturalness"`
}

func (h *Handler) getFeedback(
	ctx context.Context,
	answer, targetWord, lang, exType string,
	sentence *string,
) (string, float64, FeedbackDetail, error) {
	if h.claudeKey == "" {
		return "AI feedback unavailable (no API key configured).", 0, FeedbackDetail{}, nil
	}

	langName := map[string]string{
		"ja":    "Japanese",
		"zh-cn": "Simplified Chinese",
		"zh-tw": "Traditional Chinese",
		"ko":    "Korean",
	}[lang]
	if langName == "" {
		langName = lang
	}

	var systemPrompt string
	switch exType {
	case "writing":
		systemPrompt = fmt.Sprintf(
			`You are a %s language tutor. The student was asked to write a sentence using the word "%s".
Evaluate their answer on grammar, vocabulary use, and naturalness.
Respond with JSON only: {"score": 0-100, "feedback": "one sentence summary", "grammar": "grammar note", "vocabulary": "vocabulary note", "naturalness": "naturalness note"}`,
			langName, targetWord,
		)
	case "cloze":
		ctx_sent := ""
		if sentence != nil {
			ctx_sent = fmt.Sprintf(" The original sentence was: %s", *sentence)
		}
		systemPrompt = fmt.Sprintf(
			`You are a %s language tutor. The student was given a cloze exercise with the word "%s" blanked out.%s
Evaluate whether their answer is correct and natural.
Respond with JSON only: {"score": 0-100, "feedback": "one sentence summary", "grammar": "grammar note", "vocabulary": "vocabulary note", "naturalness": "naturalness note"}`,
			langName, targetWord, ctx_sent,
		)
	default:
		systemPrompt = fmt.Sprintf(
			`You are a %s language tutor. Evaluate this student answer for the word "%s".
Respond with JSON only: {"score": 0-100, "feedback": "one sentence", "grammar": "...", "vocabulary": "...", "naturalness": "..."}`,
			langName, targetWord,
		)
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 256,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": answer},
		},
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.claudeURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", 0, FeedbackDetail{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", h.claudeKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", 0, FeedbackDetail{}, err
	}
	defer resp.Body.Close()

	var apiResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil || len(apiResp.Content) == 0 {
		return "", 0, FeedbackDetail{}, fmt.Errorf("bad claude response")
	}

	text := apiResp.Content[0].Text

	var parsed struct {
		Score       float64 `json:"score"`
		Feedback    string  `json:"feedback"`
		Grammar     string  `json:"grammar"`
		Vocabulary  string  `json:"vocabulary"`
		Naturalness string  `json:"naturalness"`
	}
	// Extract JSON from the response (Claude sometimes adds preamble)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		json.Unmarshal([]byte(text[start:end+1]), &parsed)
	}

	detail := FeedbackDetail{
		Grammar:     parsed.Grammar,
		Vocabulary:  parsed.Vocabulary,
		Naturalness: parsed.Naturalness,
	}

	return parsed.Feedback, parsed.Score, detail, nil
}

// ── GET /v1/output/shadowing ──────────────────────────────────────────────────
// Returns the user's shadowing queue ordered by position.

func (h *Handler) ShadowingQueue(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	lang := r.URL.Query().Get("language")
	if lang == "" {
		lang = "ja"
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT sq.id, sq.audio_url, sq.transcript, sq.times_shadowed,
		        sq.last_shadowed_at, c.front_text
		 FROM shadowing_queue sq
		 LEFT JOIN cards c ON c.id = sq.card_id
		 WHERE sq.user_id = $1 AND sq.language_code = $2
		 ORDER BY sq.position ASC, sq.created_at ASC
		 LIMIT 20`,
		claims.UserID, lang,
	)
	if err != nil {
		slog.Error("shadowing queue", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	type Item struct {
		ID             string  `json:"id"`
		AudioURL       string  `json:"audio_url"`
		Transcript     *string `json:"transcript,omitempty"`
		TimesShadowed  int     `json:"times_shadowed"`
		LastShadowedAt *string `json:"last_shadowed_at,omitempty"`
		CardFront      *string `json:"card_front,omitempty"`
	}

	var items []Item
	for rows.Next() {
		var item Item
		var lastShadowed *time.Time
		var cardFront *string
		if err := rows.Scan(&item.ID, &item.AudioURL, &item.Transcript,
			&item.TimesShadowed, &lastShadowed, &cardFront); err != nil {
			continue
		}
		if lastShadowed != nil {
			s := lastShadowed.Format(time.RFC3339)
			item.LastShadowedAt = &s
		}
		item.CardFront = cardFront
		items = append(items, item)
	}

	if items == nil {
		items = []Item{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ── POST /v1/output/shadowing/{id}/complete ───────────────────────────────────

func (h *Handler) CompleteShadowing(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Extract ID from URL — chi middleware sets it; use a simple path parse here
	parts := strings.Split(r.URL.Path, "/")
	var id string
	for i, p := range parts {
		if p == "shadowing" && i+1 < len(parts) {
			id = parts[i+1]
		}
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}

	h.db.Exec(r.Context(),
		`UPDATE shadowing_queue
		 SET times_shadowed = times_shadowed + 1, last_shadowed_at = now()
		 WHERE id = $1 AND user_id = $2`,
		id, claims.UserID,
	)
	w.WriteHeader(http.StatusNoContent)
}
