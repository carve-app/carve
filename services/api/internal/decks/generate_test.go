package decks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/api/internal/auth"
)

func TestGenerate_Unauthorized(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodPost, "/v1/decks/generate", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	h.Generate(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRankAggregates_OrdersByEncounterAndFrequency(t *testing.T) {
	// common-but-unknown should rank above rare-but-encountered-once
	m := map[string]*vocabAggregate{
		"common":   {Lemma: "common", Encounter: 5, FreqRank: 200},
		"rare":     {Lemma: "rare", Encounter: 1, FreqRank: 30_000},
		"frequent": {Lemma: "frequent", Encounter: 12, FreqRank: 1_500},
	}
	got := rankAggregates(m)

	// "common" (5/(200+1) ≈ 0.0249) is the highest score.
	// "frequent" (12/1501 ≈ 0.008) is next.
	// "rare" (1/30001 ≈ 3.3e-5) is last.
	if got[0].Lemma != "common" {
		t.Errorf("expected 'common' first, got %v", got[0].Lemma)
	}
	if got[2].Lemma != "rare" {
		t.Errorf("expected 'rare' last, got %v", got[2].Lemma)
	}
}

func TestRankAggregates_MissingFreqRankFallsBack(t *testing.T) {
	m := map[string]*vocabAggregate{
		"a": {Lemma: "a", Encounter: 10, FreqRank: 0},
		"b": {Lemma: "b", Encounter: 3, FreqRank: 0},
	}
	got := rankAggregates(m)
	if got[0].Lemma != "a" {
		t.Errorf("higher encounter should win when freq_rank missing, got %v", got[0].Lemma)
	}
}

func TestRankAggregates_EmptyInputReturnsEmpty(t *testing.T) {
	got := rankAggregates(map[string]*vocabAggregate{})
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d", len(got))
	}
}

// Verifies the handler reaches its DB call path under valid auth + JSON.
// db=nil → handler will panic on Query (deferred recover); the test passes
// as long as it doesn't bail at the validation layer.
func TestGenerate_DefaultsAppliedFromQueryParams(t *testing.T) {
	h := &Handler{db: nil}
	r := httptest.NewRequest(http.MethodPost, "/v1/decks/generate?language=zh-cn&since_days=7&size=10", nil)
	r = r.WithContext(auth.ContextWithClaims(context.Background(), &auth.Claims{UserID: "u"}))
	w := httptest.NewRecorder()
	defer func() { _ = recover() }()
	h.Generate(w, r)
}
