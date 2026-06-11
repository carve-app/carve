package nlp

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

	"github.com/carve-app/carve/services/api/internal/audio"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExplainHandler serves the two popup-parity endpoints that don't belong on the
// proxy: an AI contextual word explanation (POST /v1/nlp/explain) and a word
// audio URL resolver (GET /v1/nlp/word-audio). It is intentionally separate
// from Proxy so it can hold a DB pool and a Claude client without colliding
// with proxy.go.
type ExplainHandler struct {
	db        *pgxpool.Pool
	claudeKey string
	claudeURL string
}

func NewExplainHandler(db *pgxpool.Pool) *ExplainHandler {
	return &ExplainHandler{
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

// languageName maps an ISO-ish code to a human-readable name for prompts.
// Falls back to the raw code if unknown.
func languageName(code string) string {
	if code == "" {
		return ""
	}
	name := map[string]string{
		"ja":    "Japanese",
		"zh-cn": "Simplified Chinese",
		"zh-tw": "Traditional Chinese",
		"ko":    "Korean",
		"en":    "English",
		"es":    "Spanish",
		"fr":    "French",
		"de":    "German",
	}[code]
	if name == "" {
		return code
	}
	return name
}

// buildExplainPrompt constructs the Claude prompt for a contextual word
// explanation. Extracted so it can be unit-tested without any network call.
func buildExplainPrompt(word, sentence, language, nativeLanguage string) string {
	target := languageName(language)
	native := languageName(nativeLanguage)
	if native == "" {
		native = "English"
	}

	var b strings.Builder
	if target != "" {
		fmt.Fprintf(&b, "Explain how the %s word \"%s\" is used", target, word)
	} else {
		fmt.Fprintf(&b, "Explain how the word \"%s\" is used", word)
	}
	if s := strings.TrimSpace(sentence); s != "" {
		fmt.Fprintf(&b, " in this sentence: \"%s\"", s)
	}
	fmt.Fprintf(&b, ". Answer for a language learner, in 1-2 sentences, in %s. Do not repeat the sentence; just explain the word's meaning and nuance in context.", native)
	return b.String()
}

// ── POST /v1/nlp/explain ──────────────────────────────────────────────────────
// Body: {word, sentence, language, native_language?} → {explanation: string|null}
//
// Returns {explanation: null} (HTTP 200) when no API key is configured or the
// upstream call fails, so the popup degrades gracefully rather than erroring.

func (h *ExplainHandler) Explain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Word           string `json:"word"`
		Sentence       string `json:"sentence"`
		Language       string `json:"language"`
		NativeLanguage string `json:"native_language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Word) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "word is required"})
		return
	}

	explanation, err := h.explainWord(r.Context(), req.Word, req.Sentence, req.Language, req.NativeLanguage)
	if err != nil {
		slog.Warn("explain word failed", "error", err)
		writeJSON(w, http.StatusOK, map[string]any{"explanation": nil})
		return
	}
	if explanation == "" {
		writeJSON(w, http.StatusOK, map[string]any{"explanation": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"explanation": explanation})
}

// explainWord calls Claude with the contextual-explanation prompt. Returns an
// empty string (no error) when no API key is configured — the no-key graceful
// path — and an error only on a genuine upstream failure.
func (h *ExplainHandler) explainWord(ctx context.Context, word, sentence, language, nativeLanguage string) (string, error) {
	if h.claudeKey == "" {
		return "", nil
	}

	prompt := buildExplainPrompt(word, sentence, language, nativeLanguage)

	reqBody, _ := json.Marshal(map[string]any{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 200,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.claudeURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", h.claudeKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var apiResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil || len(apiResp.Content) == 0 {
		return "", fmt.Errorf("bad claude response")
	}

	return strings.TrimSpace(apiResp.Content[0].Text), nil
}

// ── GET /v1/nlp/word-audio ────────────────────────────────────────────────────
// Query: ?language=&lemma=&reading= → {audio_url: string|null}
//
// Resolves a word-audio URL via the shared audio resolver (currently
// JapanesePod101 for Japanese). Returns {audio_url: null} when nothing is found
// rather than erroring.

func (h *ExplainHandler) WordAudio(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	language := q.Get("language")
	lemma := q.Get("lemma")
	reading := q.Get("reading")

	if language == "" {
		language = "ja"
	}

	audioURL := audio.Lookup(r.Context(), h.db, language, lemma, reading)
	if audioURL == "" {
		writeJSON(w, http.StatusOK, map[string]any{"audio_url": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"audio_url": audioURL})
}
