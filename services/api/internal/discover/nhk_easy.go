// Package discover ingests third-party reading material and ranks it by
// per-user comprehension %, the "content discovery engine" Phase 6 deliverable.
package discover

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	nhkEasyListURL    = "https://www3.nhk.or.jp/news/easy/news-list.json"
	nhkEasyArticleFmt = "https://www3.nhk.or.jp/news/easy/%s/%s.html"
	httpUserAgent     = "Carve/1.0 (+https://carve.app)"
)

// NHKArticle is one parsed-and-fetched NHK Easy article ready for ingestion.
type NHKArticle struct {
	NewsID      string
	Title       string
	Summary     string
	URL         string
	Body        string
	PublishedAt time.Time
}

// nhkListEntry mirrors a single record in news-list.json. NHK groups entries by
// date with a key like "2024-01-15"; each value is an array of entries.
type nhkListEntry struct {
	NewsID            string `json:"news_id"`
	Title             string `json:"title"`
	Outline           string `json:"outline"`
	NewsPrearrangedAt string `json:"news_prearranged_time"`
}

// FetchNHKList downloads and parses the news list, returning the most recent
// `max` articles (de-duplicated by news_id, newest first).
func FetchNHKList(client *http.Client, max int) ([]nhkListEntry, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequest(http.MethodGet, nhkEasyListURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", httpUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nhk easy list: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseNHKList(body, max)
}

// parseNHKList accepts the raw JSON body and returns up to `max` entries.
//
// The NHK feed is a 1-element array containing a single object whose keys are
// YYYY-MM-DD strings sorted newest-first; values are arrays of articles.
func parseNHKList(raw []byte, max int) ([]nhkListEntry, error) {
	var wrapper []map[string][]nhkListEntry
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("nhk list decode: %w", err)
	}
	if len(wrapper) == 0 {
		return nil, nil
	}
	dayMap := wrapper[0]

	// Sort dates descending. JSON object iteration order in Go is randomised,
	// so collect keys first then sort.
	dates := make([]string, 0, len(dayMap))
	for k := range dayMap {
		dates = append(dates, k)
	}
	// Lexical sort works since the format is YYYY-MM-DD.
	for i := 1; i < len(dates); i++ {
		for j := i; j > 0 && dates[j] > dates[j-1]; j-- {
			dates[j], dates[j-1] = dates[j-1], dates[j]
		}
	}

	out := make([]nhkListEntry, 0, max)
	seen := make(map[string]bool)
	for _, d := range dates {
		for _, e := range dayMap[d] {
			if e.NewsID == "" || seen[e.NewsID] {
				continue
			}
			seen[e.NewsID] = true
			out = append(out, e)
			if max > 0 && len(out) >= max {
				return out, nil
			}
		}
	}
	return out, nil
}

// FetchNHKArticleBody downloads an article HTML page and returns just the
// readable Japanese body text — no ruby annotations, no chrome/nav.
func FetchNHKArticleBody(client *http.Client, newsID string) (string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	url := fmt.Sprintf(nhkEasyArticleFmt, newsID, newsID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", httpUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("nhk easy article %s: status %d", newsID, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return extractNHKBody(string(body)), nil
}

var (
	// Body wrapper varies between NHK Easy templates. The current one uses
	// <div class="article-main"> wrapping <div class="article-main__body">.
	nhkBodyRe = regexp.MustCompile(`(?s)<div[^>]*class="[^"]*article-main(?:__body)?[^"]*"[^>]*>(.*?)</div>\s*</div>`)
	rubyRtRe  = regexp.MustCompile(`(?s)<rt[^>]*>.*?</rt>`)
	htmlTagRe = regexp.MustCompile(`(?s)<[^>]+>`)
	scriptRe  = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
	styleRe   = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
	wsRunRe   = regexp.MustCompile(`[ \t\r\n]+`)
)

// extractNHKBody strips ruby annotations and tags, leaving just the kanji+kana
// body text. Public so tests can hit it with fixture HTML.
func extractNHKBody(html string) string {
	// Try the body wrapper first; fall back to the whole document.
	region := html
	if m := nhkBodyRe.FindStringSubmatch(html); len(m) > 1 {
		region = m[1]
	}
	region = scriptRe.ReplaceAllString(region, " ")
	region = styleRe.ReplaceAllString(region, " ")
	// <rt> spans hold the furigana reading — keep the base text, drop the rt.
	region = rubyRtRe.ReplaceAllString(region, "")
	// Drop all remaining tags.
	region = htmlTagRe.ReplaceAllString(region, " ")
	// Decode common HTML entities.
	region = strings.ReplaceAll(region, "&nbsp;", " ")
	region = strings.ReplaceAll(region, "&amp;", "&")
	region = strings.ReplaceAll(region, "&lt;", "<")
	region = strings.ReplaceAll(region, "&gt;", ">")
	region = strings.ReplaceAll(region, "&quot;", "\"")
	// Collapse whitespace and trim.
	region = wsRunRe.ReplaceAllString(region, " ")
	return strings.TrimSpace(region)
}

// FetchNHKArticles is the convenience composite: list + body + parsed time.
func FetchNHKArticles(client *http.Client, max int) ([]NHKArticle, error) {
	entries, err := FetchNHKList(client, max)
	if err != nil {
		return nil, err
	}
	out := make([]NHKArticle, 0, len(entries))
	for _, e := range entries {
		body, err := FetchNHKArticleBody(client, e.NewsID)
		if err != nil {
			// Skip individual failures — partial ingestion beats none.
			continue
		}
		pub, _ := time.Parse("2006-01-02 15:04:05", e.NewsPrearrangedAt)
		out = append(out, NHKArticle{
			NewsID:      e.NewsID,
			Title:       stripRubyAndTags(e.Title),
			Summary:     stripRubyAndTags(e.Outline),
			URL:         fmt.Sprintf(nhkEasyArticleFmt, e.NewsID, e.NewsID),
			Body:        body,
			PublishedAt: pub,
		})
	}
	return out, nil
}

// stripRubyAndTags handles title fields that come pre-annotated with ruby.
func stripRubyAndTags(s string) string {
	s = rubyRtRe.ReplaceAllString(s, "")
	s = htmlTagRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}
