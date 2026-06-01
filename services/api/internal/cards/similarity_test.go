package cards

import (
	"math"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestCharTrigrams_EmptyReturnsNil(t *testing.T) {
	if got := charTrigrams(""); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	if got := charTrigrams("   "); got != nil {
		t.Fatalf("whitespace-only should normalise to empty, got %v", got)
	}
}

func TestCharTrigrams_PadsShortStrings(t *testing.T) {
	got := charTrigrams("ab")
	if len(got) != 1 {
		t.Fatalf("short string should produce one key, got %d", len(got))
	}
	if _, ok := got[" ab"]; !ok {
		t.Fatalf("expected ' ab' key, got %v", got)
	}
}

func TestCharTrigrams_BasicAscii(t *testing.T) {
	got := charTrigrams("hello")
	want := []string{"hel", "ell", "llo"}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("missing trigram %q in %v", w, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("expected 3 trigrams, got %d", len(got))
	}
}

func TestCharTrigrams_CollapsesWhitespace(t *testing.T) {
	a := charTrigrams("hello world")
	b := charTrigrams("hello   world")
	c := charTrigrams("hello\tworld")
	if !approx(jaccardTrigrams(a, b), 1.0) {
		t.Errorf("collapsed-ws variants should be identical sets")
	}
	if !approx(jaccardTrigrams(a, c), 1.0) {
		t.Errorf("tab and space should normalise the same")
	}
}

func TestCharTrigrams_CJK(t *testing.T) {
	got := charTrigrams("私は寿司を食べる")
	if len(got) == 0 {
		t.Fatal("CJK string should produce trigrams")
	}
	// 8 runes → 6 trigrams
	if len(got) != 6 {
		t.Errorf("expected 6 trigrams, got %d", len(got))
	}
}

func TestJaccard_IdenticalSentences(t *testing.T) {
	a := charTrigrams("私は寿司を食べる。")
	b := charTrigrams("私は寿司を食べる。")
	if got := jaccardTrigrams(a, b); !approx(got, 1.0) {
		t.Errorf("identical → 1.0, got %v", got)
	}
}

func TestJaccard_DisjointSentences(t *testing.T) {
	a := charTrigrams("abcdef")
	b := charTrigrams("xyzwvu")
	if got := jaccardTrigrams(a, b); got != 0 {
		t.Errorf("disjoint → 0, got %v", got)
	}
}

func TestJaccard_PartialOverlap_CJK(t *testing.T) {
	// Two sentences sharing several characters
	a := charTrigrams("私は毎日寿司を食べる")
	b := charTrigrams("彼は毎日寿司を食べる")
	got := jaccardTrigrams(a, b)
	if got <= 0.5 || got >= 1.0 {
		t.Errorf("expected partial overlap (0.5,1.0), got %v", got)
	}
}

func TestJaccard_EmptyHandling(t *testing.T) {
	a := charTrigrams("abc")
	if got := jaccardTrigrams(a, nil); got != 0 {
		t.Errorf("nil set → 0, got %v", got)
	}
	if got := jaccardTrigrams(nil, nil); got != 0 {
		t.Errorf("both nil → 0, got %v", got)
	}
}

func TestJaccard_CaseInsensitive(t *testing.T) {
	a := charTrigrams("Hello World")
	b := charTrigrams("hello world")
	if got := jaccardTrigrams(a, b); !approx(got, 1.0) {
		t.Errorf("case differences should be normalised, got %v", got)
	}
}
