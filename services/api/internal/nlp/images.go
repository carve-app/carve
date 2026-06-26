package nlp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// images.go implements a keyless, best-effort dictionary-image lookup for the
// word-lookup popup.
//
// Source: the Wikipedia REST summary API
//   https://<lang>.wikipedia.org/api/rest_v1/page/summary/<word>
// We return its thumbnail.source if present. This was chosen over Openverse
// because it is fully keyless, needs no account, has a stable per-language
// host, and returns a single canonical thumbnail for the exact term.
//
// HONEST CAVEAT: image relevance is best-effort. Wikipedia matches on the
// page title, so a word that is also a common page (e.g. a place name or a
// disambiguation page) may return an unrelated or generic image, and many
// words have no page at all (we return null in that case). The image is
// meant as a memory aid, not an authoritative illustration.

// wikiHostForLanguage maps an app language code to the Wikipedia subdomain.
// Unknown languages fall back to English.
func wikiHostForLanguage(language string) string {
	switch language {
	case "ja", "en", "es", "de", "fr", "it", "pt", "zh", "ko":
		return language + ".wikipedia.org"
	default:
		return "en.wikipedia.org"
	}
}

// wikiSummaryBaseURL is the scheme+host used to build the summary request. It
// is a field on the Proxy so tests can point it at an httptest server. When
// empty, the real per-language Wikipedia host is used.
//
// (Stored on Proxy via wikiBaseOverride below to avoid changing the struct's
// public surface; see WordImage.)

// wikiSummary models the subset of the Wikipedia REST summary payload we use.
type wikiSummary struct {
	Thumbnail struct {
		Source string `json:"source"`
	} `json:"thumbnail"`
}

// wordImageBaseOverride lets tests substitute the Wikipedia base URL
// ("scheme://host", no trailing slash). Empty means use the real host.
var wordImageBaseOverride string

// imageWriteJSON writes a JSON response. Kept local to this file so it does
// not collide with helpers in other packages; the nlp package has none.
func imageWriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WordImage handles GET /v1/nlp/word-image?word=&language=
//
// Returns {"image_url": string|null}. Always responds 200 with a null
// image_url on any failure (missing word, no Wikipedia page, network error,
// timeout) so the popup can treat it as a purely optional enhancement.
func (p *Proxy) WordImage(w http.ResponseWriter, r *http.Request) {
	word := r.URL.Query().Get("word")
	language := r.URL.Query().Get("language")

	if word == "" {
		imageWriteJSON(w, http.StatusOK, map[string]any{"image_url": nil})
		return
	}

	src := p.fetchWikipediaThumbnail(r.Context(), word, language)
	if src == "" {
		imageWriteJSON(w, http.StatusOK, map[string]any{"image_url": nil})
		return
	}
	imageWriteJSON(w, http.StatusOK, map[string]any{"image_url": src})
}

// fetchWikipediaThumbnail returns the thumbnail URL for `word` from the
// language-appropriate Wikipedia, or "" on any failure. Times out fast (5s).
func (p *Proxy) fetchWikipediaThumbnail(parent context.Context, word, language string) string {
	base := wordImageBaseOverride
	if base == "" {
		base = "https://" + wikiHostForLanguage(language)
	}

	// Wikipedia titles use underscores for spaces; PathEscape handles the rest.
	title := url.PathEscape(word)
	reqURL := fmt.Sprintf("%s/api/rest_v1/page/summary/%s", base, title)

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		slog.Debug("word-image: build request failed", "error", err)
		return ""
	}
	// Wikipedia REST API requests a descriptive User-Agent.
	req.Header.Set("User-Agent", "CarveApp/1.0 (language-learning; word-image lookup)")
	req.Header.Set("Accept", "application/json")

	resp, err := p.do(req)
	if err != nil {
		slog.Debug("word-image: upstream request failed", "url", reqURL, "error", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 404 = no such page; anything else = transient. Either way, null.
		return ""
	}

	var summary wikiSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		slog.Debug("word-image: decode failed", "error", err)
		return ""
	}
	return summary.Thumbnail.Source
}
