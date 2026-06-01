package discover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/carve-app/carve/services/api/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Ingester pulls fresh articles from discovery sources and persists them with
// pre-computed content-lemma sets, so per-user comprehension can be calculated
// at query time without re-tokenising.
type Ingester struct {
	db         *pgxpool.Pool
	nlpBaseURL string
	httpClient *http.Client
}

func NewIngester(db *pgxpool.Pool) *Ingester {
	nlp := os.Getenv("NLP_SERVICE_URL")
	if nlp == "" {
		nlp = "http://localhost:8001"
	}
	return &Ingester{
		db:         db,
		nlpBaseURL: nlp,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// IngestNHKEasy is the public entry point — typically called by the 6-hour
// cron in cmd/api/main.go. It fetches up to `max` recent NHK Easy articles,
// tokenises any that aren't already in the discover_articles table, and
// inserts them.
func (i *Ingester) IngestNHKEasy(ctx context.Context, max int) (int, error) {
	if max <= 0 {
		max = 30
	}
	articles, err := FetchNHKArticles(i.httpClient, max)
	if err != nil {
		return 0, fmt.Errorf("fetch nhk articles: %w", err)
	}

	inserted := 0
	for _, a := range articles {
		if a.Body == "" {
			continue
		}
		// Skip if we already have this URL.
		var exists bool
		if err := i.db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM discover_articles WHERE url = $1)`,
			a.URL,
		).Scan(&exists); err != nil {
			slog.Warn("discover: existence check failed", "url", a.URL, "error", err)
			continue
		}
		if exists {
			continue
		}

		lemmas, contentCount, err := i.tokenizeContentLemmas(ctx, a.Body, "ja")
		if err != nil {
			slog.Warn("discover: tokenize failed; skipping", "url", a.URL, "error", err)
			continue
		}

		id := auth.NewID()
		var publishedAt any
		if !a.PublishedAt.IsZero() {
			publishedAt = a.PublishedAt
		}

		if _, err := i.db.Exec(ctx,
			`INSERT INTO discover_articles
			    (id, source, language_code, title, summary, body, url,
			     content_lemmas, total_content_words, published_at)
			 VALUES ($1, 'nhk-easy', 'ja', $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (url) DO NOTHING`,
			id, a.Title, a.Summary, a.Body, a.URL, lemmas, contentCount, publishedAt,
		); err != nil {
			slog.Warn("discover: insert failed", "url", a.URL, "error", err)
			continue
		}
		inserted++
	}
	slog.Info("discover: nhk easy ingested", "inserted", inserted, "checked", len(articles))
	return inserted, nil
}

// tokenizeContentLemmas hits the NLP /tokenize endpoint and returns the
// deduped sorted list of content-word lemmas plus the total content-word
// count (with duplicates) so comprehension % can be calculated as the
// fraction of content words whose lemma is in a user's known/learning set.
func (i *Ingester) tokenizeContentLemmas(ctx context.Context, text, language string) ([]string, int, error) {
	body, _ := json.Marshal(map[string]any{
		"text":     text,
		"language": language,
	})
	tokenCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(tokenCtx, http.MethodPost,
		i.nlpBaseURL+"/tokenize", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret := os.Getenv("NLP_INTERNAL_SECRET"); secret != "" {
		req.Header.Set("X-Internal-Secret", secret)
	}
	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("tokenize: status %d", resp.StatusCode)
	}

	var result struct {
		Tokens []struct {
			Lemma         string `json:"lemma"`
			IsContentWord bool   `json:"is_content_word"`
		} `json:"tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, err
	}

	seen := make(map[string]struct{})
	contentCount := 0
	for _, t := range result.Tokens {
		if !t.IsContentWord || t.Lemma == "" {
			continue
		}
		contentCount++
		seen[t.Lemma] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	sort.Strings(out)
	return out, contentCount, nil
}
