package discover

import (
	"strings"
	"testing"
	"time"
)

func TestParseRSS_Basic(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Test feed</title>
    <item>
      <title>One</title>
      <link>https://example.com/a</link>
      <description>Hello world</description>
      <pubDate>Wed, 14 Feb 2024 09:00:00 +0900</pubDate>
    </item>
    <item>
      <title>Two</title>
      <link>https://example.com/b</link>
      <content:encoded><![CDATA[<p>これは<ruby>本文<rt>ほんぶん</rt></ruby>です。</p>]]></content:encoded>
      <description>fallback</description>
      <pubDate>Tue, 13 Feb 2024 09:00:00 +0900</pubDate>
    </item>
  </channel>
</rss>`)
	got, err := parseRSS(raw, 10)
	if err != nil {
		t.Fatalf("parseRSS: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(got))
	}

	if got[0].Title != "One" {
		t.Errorf("title=%q", got[0].Title)
	}
	if got[0].URL != "https://example.com/a" {
		t.Errorf("url=%q", got[0].URL)
	}
	if !strings.Contains(got[0].Body, "Hello world") {
		t.Errorf("body=%q", got[0].Body)
	}

	// Second article prefers content:encoded over description.
	if !strings.Contains(got[1].Body, "本文") {
		t.Errorf("expected 本文 in body, got %q", got[1].Body)
	}
	if strings.Contains(got[1].Body, "ほんぶん") {
		t.Errorf("ruby reading should be stripped, got %q", got[1].Body)
	}
	if strings.Contains(got[1].Body, "fallback") {
		t.Errorf("description shouldn't be used when encoded present, got %q", got[1].Body)
	}
}

func TestParseRSS_Caps(t *testing.T) {
	raw := []byte(`<rss><channel>
		<item><title>1</title><link>a</link><description>d</description></item>
		<item><title>2</title><link>b</link><description>d</description></item>
		<item><title>3</title><link>c</link><description>d</description></item>
	</channel></rss>`)
	got, _ := parseRSS(raw, 2)
	if len(got) != 2 {
		t.Errorf("max=2 should cap, got %d", len(got))
	}
}

func TestParseRSS_SkipsEmptyBodies(t *testing.T) {
	raw := []byte(`<rss><channel>
		<item><title>kept</title><link>a</link><description>body</description></item>
		<item><title>dropped</title><link>b</link><description></description></item>
	</channel></rss>`)
	got, _ := parseRSS(raw, 10)
	if len(got) != 1 || got[0].Title != "kept" {
		t.Errorf("empty body should be skipped, got %+v", got)
	}
}

func TestParsePubDate_AcceptsCommonFormats(t *testing.T) {
	cases := []string{
		"Wed, 14 Feb 2024 09:00:00 +0900",
		"Wed, 14 Feb 2024 09:00:00 JST",
		"2024-02-14T09:00:00Z",
	}
	for _, c := range cases {
		got := parsePubDate(c)
		if got.IsZero() {
			t.Errorf("expected to parse %q", c)
		}
		if got.Year() != 2024 {
			t.Errorf("%q → wrong year %d", c, got.Year())
		}
	}
}

func TestParsePubDate_ZeroOnGarbage(t *testing.T) {
	if !parsePubDate("not a date").IsZero() {
		t.Error("expected zero time on garbage")
	}
	if !parsePubDate("").IsZero() {
		t.Error("expected zero time on empty")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("no-op truncate: %q", got)
	}
	if got := truncate("こんにちは世界", 3); got != "こんに…" {
		t.Errorf("rune truncate: %q", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "  ", "a", "b"); got != "a" {
		t.Errorf("got %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestRSSArticle_PreservesPubDate(t *testing.T) {
	raw := []byte(`<rss><channel><item>
		<title>t</title><link>https://x.example/post</link>
		<description>body</description>
		<pubDate>Mon, 15 Apr 2024 12:00:00 +0000</pubDate>
	</item></channel></rss>`)
	got, _ := parseRSS(raw, 10)
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].PublishedAt.Year() != 2024 || got[0].PublishedAt.Month() != time.April {
		t.Errorf("pubDate not preserved: %v", got[0].PublishedAt)
	}
}
