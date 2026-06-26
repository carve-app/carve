package nlp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Proxy forwards NLP requests to the Python NLP service.
type Proxy struct {
	serviceURL     string
	internalSecret string
	client         HTTPDoer
	db             *pgxpool.Pool
}

// HTTPDoer is the provider boundary used by NLP and dictionary-image calls.
// Tests inject deterministic timeout/malformed-response transports without
// requiring a real Python service or public provider.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func NewProxy(db ...*pgxpool.Pool) *Proxy {
	url := os.Getenv("NLP_SERVICE_URL")
	if url == "" {
		url = "http://localhost:8001"
	}
	p := &Proxy{
		serviceURL:     url,
		internalSecret: os.Getenv("NLP_INTERNAL_SECRET"),
		client:         http.DefaultClient,
	}
	if len(db) > 0 {
		p.db = db[0]
	}
	return p
}

func NewProxyWithClient(serviceURL, internalSecret string, client HTTPDoer) *Proxy {
	if client == nil {
		client = http.DefaultClient
	}
	return &Proxy{serviceURL: serviceURL, internalSecret: internalSecret, client: client}
}

func (p *Proxy) do(req *http.Request) (*http.Response, error) {
	if p.client == nil {
		return http.DefaultClient.Do(req)
	}
	return p.client.Do(req)
}

const nlpTimeout = 120 * time.Second

const nlpMaxResponseBytes = 10 << 20

func writeUpstreamResponse(w http.ResponseWriter, resp *http.Response) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, nlpMaxResponseBytes+1))
	if err != nil {
		slog.Error("nlp proxy: read response body", "error", err)
		http.Error(w, `{"error":"invalid nlp service response"}`, http.StatusBadGateway)
		return
	}
	if len(body) > nlpMaxResponseBytes {
		http.Error(w, `{"error":"nlp service response too large"}`, http.StatusBadGateway)
		return
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") && !json.Valid(body) {
		http.Error(w, `{"error":"invalid nlp service response"}`, http.StatusBadGateway)
		return
	}
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(body); err != nil {
		slog.Error("nlp proxy: write response body", "error", err)
	}
}

// forward proxies the incoming request body to the given upstream path,
// copies all response headers and the body back to the client.
func (p *Proxy) forward(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	upstreamURL := p.serviceURL + upstreamPath

	// Use a dedicated timeout longer than the global chi middleware (30s) so
	// the first request after NLP service startup isn't killed prematurely.
	ctx, cancel := context.WithTimeout(r.Context(), nlpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, r.Body)
	if err != nil {
		slog.Error("nlp proxy: build request", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Forward content-type from the original request.
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}

	// Attach internal shared secret if configured.
	if p.internalSecret != "" {
		req.Header.Set("X-Internal-Secret", p.internalSecret)
	}

	resp, err := p.do(req)
	if err != nil {
		slog.Error("nlp proxy: upstream request failed", "url", upstreamURL, "error", err)
		http.Error(w, `{"error":"nlp service unavailable"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	writeUpstreamResponse(w, resp)
}

// POST /v1/nlp/tokenize
func (p *Proxy) Tokenize(w http.ResponseWriter, r *http.Request) {
	p.forwardWithKnowledge(w, r, "/tokenize")
}

// POST /v1/nlp/lookup
func (p *Proxy) Lookup(w http.ResponseWriter, r *http.Request) {
	p.forward(w, r, "/lookup")
}

// POST /v1/nlp/score-content — proxies to NLP /score-text.
func (p *Proxy) ScoreContent(w http.ResponseWriter, r *http.Request) {
	p.forwardWithKnowledge(w, r, "/score-text")
}

// forwardWithKnowledge merges vocabulary persisted by onboarding/imports and
// mined cards into the caller-provided status lists. This makes every first-
// party tokenizer/scorer request reflect the same server-side vocabulary even
// when a browser has just been installed and has no local cache yet.
func (p *Proxy) forwardWithKnowledge(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	if p.db == nil {
		p.forward(w, r, upstreamPath)
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		// Preserve the upstream API's validation/error shape for malformed JSON.
		r.Body = io.NopCloser(bytes.NewReader(raw))
		p.forward(w, r, upstreamPath)
		return
	}

	claims, ok := auth.ClaimsFromContext(r.Context())
	language, _ := payload["language"].(string)
	if ok && language != "" {
		known, learning := p.loadUserKnowledge(r.Context(), claims.UserID, language)
		payload["known_lemmas"] = mergeStrings(payload["known_lemmas"], known, nil)
		knownSet := make(map[string]bool, len(known))
		for _, lemma := range known {
			knownSet[lemma] = true
		}
		payload["learning_lemmas"] = mergeStrings(payload["learning_lemmas"], learning, knownSet)
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(encoded))
	r.ContentLength = int64(len(encoded))
	p.forward(w, r, upstreamPath)
}

func (p *Proxy) loadUserKnowledge(ctx context.Context, userID, language string) (known, learning []string) {
	rows, err := p.db.Query(ctx,
		`SELECT lemma, status FROM (
		   SELECT w.lemma, uwk.status
		   FROM user_word_knowledge uwk
		   JOIN words w ON w.id = uwk.word_id
		   WHERE uwk.user_id = $1 AND w.language_code = $2
		   UNION ALL
		   SELECT c.front_text,
		          CASE WHEN c.fsrs_state = 'review' THEN 'known' ELSE 'learning' END
		   FROM cards c
		   WHERE c.user_id = $1 AND c.language_code = $2 AND c.deleted_at IS NULL
		 ) vocab`,
		userID, language,
	)
	if err != nil {
		slog.Warn("nlp proxy: load user knowledge", "error", err)
		return nil, nil
	}
	defer rows.Close()
	knownSet := map[string]bool{}
	learningSet := map[string]bool{}
	for rows.Next() {
		var lemma, status string
		if rows.Scan(&lemma, &status) != nil || lemma == "" {
			continue
		}
		if status == "known" {
			knownSet[lemma] = true
			delete(learningSet, lemma)
		} else if !knownSet[lemma] {
			learningSet[lemma] = true
		}
	}
	for lemma := range knownSet {
		known = append(known, lemma)
	}
	for lemma := range learningSet {
		learning = append(learning, lemma)
	}
	return known, learning
}

func mergeStrings(existing any, additions []string, excluded map[string]bool) []string {
	seen := map[string]bool{}
	out := []string{}
	appendOne := func(value string) {
		if value == "" || seen[value] || (excluded != nil && excluded[value]) {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	if values, ok := existing.([]any); ok {
		for _, value := range values {
			if text, ok := value.(string); ok {
				appendOne(text)
			}
		}
	}
	for _, value := range additions {
		appendOne(value)
	}
	return out
}

// POST /v1/nlp/translate — proxies to NLP /translate.
// Returns {translation: null} if the upstream is unavailable.
func (p *Proxy) Translate(w http.ResponseWriter, r *http.Request) {
	p.forward(w, r, "/translate")
}

// POST /v1/nlp/select-sentence — proxies to NLP /select-sentence.
// Returns the best i+1 candidate sentence for mining the requested word.
func (p *Proxy) SelectSentence(w http.ResponseWriter, r *http.Request) {
	p.forward(w, r, "/select-sentence")
}

// GET /v1/nlp/grammar/patterns — proxies to NLP /grammar/patterns.
// Lists all detectable grammar patterns for the requested language.
func (p *Proxy) GrammarPatterns(w http.ResponseWriter, r *http.Request) {
	upstreamPath := "/grammar/patterns"
	if q := r.URL.RawQuery; q != "" {
		upstreamPath += "?" + q
	}
	p.forwardGET(w, r, upstreamPath)
}

// forwardGET is a GET-flavoured forward (no body).
func (p *Proxy) forwardGET(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	upstreamURL := p.serviceURL + upstreamPath
	ctx, cancel := context.WithTimeout(r.Context(), nlpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstreamURL, nil)
	if err != nil {
		slog.Error("nlp proxy GET: build request", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if p.internalSecret != "" {
		req.Header.Set("X-Internal-Secret", p.internalSecret)
	}
	resp, err := p.do(req)
	if err != nil {
		slog.Error("nlp proxy GET: upstream request failed", "url", upstreamURL, "error", err)
		http.Error(w, `{"error":"nlp service unavailable"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	writeUpstreamResponse(w, resp)
}
