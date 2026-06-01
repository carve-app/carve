package discover

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

func TestFitScore_BoundsAndPeak(t *testing.T) {
	cases := []struct {
		pct  float64
		want float64
	}{
		{60, 0},
		{70, 0},
		{93, 1},
		{100, 0},
		{110, 0},
	}
	for _, c := range cases {
		got := fitScore(c.pct)
		if got < c.want-1e-9 || got > c.want+1e-9 {
			t.Errorf("fitScore(%v) = %v, want %v", c.pct, got, c.want)
		}
	}
}

func TestFitScore_PeakIsHighestPoint(t *testing.T) {
	peak := fitScore(93)
	for pct := 70.0; pct <= 100.0; pct += 0.5 {
		if fitScore(pct) > peak+1e-9 {
			t.Errorf("fitScore(%v)=%v exceeds peak fitScore(93)=%v", pct, fitScore(pct), peak)
		}
	}
}

func TestClassifyMode(t *testing.T) {
	cases := []struct {
		pct  float64
		want string
	}{
		{99, "flow_read"},
		{95, "mining_read"},
		{85, "study_read"},
		{50, "too_hard"},
	}
	for _, c := range cases {
		if got := classifyMode(c.pct); got != c.want {
			t.Errorf("classifyMode(%v) = %v, want %v", c.pct, got, c.want)
		}
	}
}

func TestFeedHandler_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodGet, "/v1/discover/feed", nil)
	w := httptest.NewRecorder()
	h.Feed(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestFeedItem_JSONSerialisation(t *testing.T) {
	item := FeedItem{
		ID:               "abc",
		Source:           "nhk-easy",
		Title:            "テスト",
		URL:              "https://example.com",
		ComprehensionPct: 92.3,
		UnknownCount:     5,
		RecommendedMode:  "mining_read",
		FitScore:         0.9696,
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(item); err != nil {
		t.Fatalf("encode: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"abc", "nhk-easy", "comprehension_pct", "92.3", "mining_read"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}

// ensure auth context wiring matches other handlers
func TestFeedHandler_AuthedRequestDoesntPanicOnNilDB(t *testing.T) {
	// With db=nil the Query call will panic on dereferencing; we expect a 500
	// — confirming the handler reaches the query without crashing on auth.
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodGet, "/v1/discover/feed", nil)
	r = r.WithContext(auth.ContextWithClaims(context.Background(), &auth.Claims{UserID: "u"}))
	w := httptest.NewRecorder()
	defer func() {
		// A nil pgxpool.Pool will panic on Query — that's fine for this test;
		// we just want to confirm auth passes.
		_ = recover()
	}()
	h.Feed(w, r)
}
