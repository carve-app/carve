package discover

import (
	"strings"
	"testing"
)

func TestParseNHKList_Empty(t *testing.T) {
	got, err := parseNHKList([]byte(`[]`), 10)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 entries, got %d", len(got))
	}
}

func TestParseNHKList_Basic(t *testing.T) {
	raw := []byte(`[{
		"2024-01-15": [
			{"news_id":"abc","title":"<ruby>春<rt>はる</rt></ruby>","outline":"out1","news_prearranged_time":"2024-01-15 09:00:00"},
			{"news_id":"def","title":"夏","outline":"out2","news_prearranged_time":"2024-01-15 10:00:00"}
		],
		"2024-01-14": [
			{"news_id":"ghi","title":"秋","outline":"out3","news_prearranged_time":"2024-01-14 09:00:00"}
		]
	}]`)
	got, err := parseNHKList(raw, 10)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	// Newest date first, original entry order within the date preserved.
	if got[0].NewsID != "abc" || got[2].NewsID != "ghi" {
		t.Errorf("expected order abc/def/ghi, got %v/%v/%v",
			got[0].NewsID, got[1].NewsID, got[2].NewsID)
	}
}

func TestParseNHKList_DedupesByNewsID(t *testing.T) {
	raw := []byte(`[{
		"2024-01-15": [{"news_id":"x","title":"a","outline":"","news_prearranged_time":""}],
		"2024-01-14": [{"news_id":"x","title":"a","outline":"","news_prearranged_time":""}]
	}]`)
	got, _ := parseNHKList(raw, 10)
	if len(got) != 1 {
		t.Errorf("dup news_id should collapse, got %d entries", len(got))
	}
}

func TestParseNHKList_Caps(t *testing.T) {
	raw := []byte(`[{
		"2024-01-15": [
			{"news_id":"a","title":"","outline":"","news_prearranged_time":""},
			{"news_id":"b","title":"","outline":"","news_prearranged_time":""},
			{"news_id":"c","title":"","outline":"","news_prearranged_time":""}
		]
	}]`)
	got, _ := parseNHKList(raw, 2)
	if len(got) != 2 {
		t.Errorf("max=2 should cap, got %d", len(got))
	}
}

func TestExtractNHKBody_StripsRubyAndTags(t *testing.T) {
	html := `<html><body>
		<div class="article-main">
		  <div class="article-main__body">
		    <p><ruby>今日<rt>きょう</rt></ruby>は<ruby>天気<rt>てんき</rt></ruby>がいいです。</p>
		    <p>明日も晴れる。</p>
		  </div>
		</div>
		<script>var x = 1;</script>
	</body></html>`
	got := extractNHKBody(html)
	if !strings.Contains(got, "今日") || !strings.Contains(got, "天気") {
		t.Errorf("expected kanji base to remain, got %q", got)
	}
	if strings.Contains(got, "きょう") || strings.Contains(got, "てんき") {
		t.Errorf("ruby readings should be stripped, got %q", got)
	}
	if strings.Contains(got, "var x") {
		t.Errorf("scripts should be stripped, got %q", got)
	}
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("HTML tags should be stripped, got %q", got)
	}
}

func TestExtractNHKBody_FallbackWhenNoWrapper(t *testing.T) {
	// Body without the expected wrapper class — fall back to whole document.
	html := `<html><body><p>テスト<rt>strip</rt></p></body></html>`
	got := extractNHKBody(html)
	if !strings.Contains(got, "テスト") {
		t.Errorf("fallback should still extract text, got %q", got)
	}
}

func TestExtractNHKBody_CollapsesWhitespace(t *testing.T) {
	html := `<div class="article-main"><div class="article-main__body">
		<p>あ\tい
		う</p>
	</div></div>`
	got := extractNHKBody(html)
	if strings.Contains(got, "  ") {
		t.Errorf("multiple spaces should collapse, got %q", got)
	}
}

func TestStripRubyAndTags(t *testing.T) {
	in := "<ruby>春<rt>はる</rt></ruby>のニュース"
	got := stripRubyAndTags(in)
	want := "春のニュース"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
