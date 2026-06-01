package discover

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// rssFeed represents the subset of RSS 2.0 we care about.
type rssFeed struct {
	XMLName xml.Name  `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title string    `xml:"title"`
	Items []rssItem `xml:"item"`
}

// content:encoded uses a namespace; XML decoder treats it as Encoded with the
// "encoded" element name. Including both Description and Encoded lets us prefer
// the richer body when available.
type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Encoded     string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

// RSSArticle is a normalised feed entry ready for ingestion. Body has had
// HTML and ruby annotations stripped.
type RSSArticle struct {
	GUID        string
	Title       string
	Summary     string
	URL         string
	Body        string
	PublishedAt time.Time
}

// FetchRSS pulls a feed URL and returns parsed articles, newest first.
func FetchRSS(client *http.Client, feedURL string, max int) ([]RSSArticle, error) {
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequest(http.MethodGet, feedURL, nil)
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
		return nil, fmt.Errorf("rss %s: status %d", feedURL, resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseRSS(raw, max)
}

// parseRSS decodes the feed bytes into RSSArticle structs.
func parseRSS(raw []byte, max int) ([]RSSArticle, error) {
	var feed rssFeed
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return nil, fmt.Errorf("rss decode: %w", err)
	}
	out := make([]RSSArticle, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		body := it.Encoded
		if strings.TrimSpace(body) == "" {
			body = it.Description
		}
		// extractNHKBody is generic enough to strip arbitrary HTML — reuse it.
		body = extractNHKBody(body)
		if body == "" {
			continue
		}
		out = append(out, RSSArticle{
			GUID:        firstNonEmpty(it.GUID, it.Link),
			Title:       stripRubyAndTags(it.Title),
			Summary:     truncate(extractNHKBody(it.Description), 220),
			URL:         it.Link,
			Body:        body,
			PublishedAt: parsePubDate(it.PubDate),
		})
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// parsePubDate handles RFC1123Z (the canonical RSS format) and a few common
// variants. Returns zero time on failure — caller can treat as "unknown".
func parsePubDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"Mon, 02 Jan 2006 15:04:05 MST",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
