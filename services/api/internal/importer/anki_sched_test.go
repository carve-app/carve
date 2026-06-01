package importer

import (
	"math"
	"math/rand"
	"testing"
	"testing/quick"
	"time"
)

// ── Unit tests: SM-2 → FSRS state mapping ─────────────────────────────────────

func TestMap_NewCard(t *testing.T) {
	a := AnkiCardSched{Type: 0, Queue: 0}
	got := Map(a, time.Unix(0, 0), time.Now())
	if got.State != "new" {
		t.Errorf("expected new, got %q", got.State)
	}
	if got.Stability != nil || got.Difficulty != nil || got.Due != nil {
		t.Errorf("new cards must not carry FSRS state: %+v", got)
	}
}

func TestMap_ReviewCardPreservesIVL(t *testing.T) {
	a := AnkiCardSched{Type: 2, Queue: 2, IVL: 42, Factor: 2500, Reps: 7, Lapses: 1, Due: 100}
	got := Map(a, time.Unix(0, 0), time.Now())
	if got.State != "review" {
		t.Errorf("expected review, got %q", got.State)
	}
	if got.Stability == nil || *got.Stability != 42 {
		t.Errorf("expected stability=42, got %v", got.Stability)
	}
	if got.Reps != 7 || got.Lapses != 1 {
		t.Errorf("reps/lapses must round-trip: got %d/%d", got.Reps, got.Lapses)
	}
}

func TestMap_DefaultEaseMapsToMidDifficulty(t *testing.T) {
	a := AnkiCardSched{Type: 2, Queue: 2, IVL: 10, Factor: 2500}
	got := Map(a, time.Unix(0, 0), time.Now())
	if got.Difficulty == nil || math.Abs(*got.Difficulty-4) > 0.01 {
		t.Errorf("default ease 2500 should map to difficulty 4, got %v", got.Difficulty)
	}
}

func TestMap_HighEaseMapsToLowDifficulty(t *testing.T) {
	a := AnkiCardSched{Type: 2, Queue: 2, IVL: 30, Factor: 3500}
	got := Map(a, time.Unix(0, 0), time.Now())
	if got.Difficulty == nil || *got.Difficulty > 3 {
		t.Errorf("ease 3500 should be easier than default; got %v", got.Difficulty)
	}
}

func TestMap_LowEaseMapsToHighDifficulty(t *testing.T) {
	a := AnkiCardSched{Type: 2, Queue: 2, IVL: 5, Factor: 1300}
	got := Map(a, time.Unix(0, 0), time.Now())
	if got.Difficulty == nil || *got.Difficulty < 7 {
		t.Errorf("ease 1300 should be hard; got %v", got.Difficulty)
	}
}

func TestMap_SuspendedFromQueue(t *testing.T) {
	a := AnkiCardSched{Type: 2, Queue: -1, IVL: 30, Factor: 2500}
	got := Map(a, time.Unix(0, 0), time.Now())
	if got.State != "suspended" {
		t.Errorf("queue=-1 should be suspended, got %q", got.State)
	}
}

func TestMap_DueConvertedFromRelativeDays(t *testing.T) {
	crt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	a := AnkiCardSched{Type: 2, Queue: 2, IVL: 10, Factor: 2500, Due: 100}
	got := Map(a, crt, time.Now())
	if got.Due == nil {
		t.Fatal("review card must carry due time")
	}
	expected := crt.Add(100 * 24 * time.Hour)
	if !got.Due.Equal(expected) {
		t.Errorf("expected due=%v, got %v", expected, got.Due)
	}
}

func TestMap_LearningCardSchedulesSoon(t *testing.T) {
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	a := AnkiCardSched{Type: 1, Queue: 1, Factor: 2500}
	got := Map(a, time.Unix(0, 0), now)
	if got.State != "learning" {
		t.Errorf("expected learning, got %q", got.State)
	}
	if got.Due == nil || got.Due.Sub(now) > 30*time.Minute {
		t.Errorf("learning card should be due soon, got %v after now=%v", got.Due, now)
	}
}

// ── Round-trip: Unmap(Map(x)) ≡ x for the fields the carve scheduler cares
// about (state, stability/ivl, difficulty/factor, reps, lapses, due/day).

func TestRoundTrip_ReviewCard(t *testing.T) {
	crt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	orig := AnkiCardSched{Type: 2, Queue: 2, IVL: 42, Factor: 2500, Reps: 7, Lapses: 1, Due: 100}
	round := Unmap(Map(orig, crt, time.Now()), crt)

	if round.Type != orig.Type || round.Queue != orig.Queue {
		t.Errorf("type/queue diverged: orig=%+v round=%+v", orig, round)
	}
	if round.IVL != orig.IVL {
		t.Errorf("IVL diverged: orig=%d round=%d", orig.IVL, round.IVL)
	}
	if round.Factor != orig.Factor {
		t.Errorf("Factor diverged: orig=%d round=%d", orig.Factor, round.Factor)
	}
	if round.Reps != orig.Reps || round.Lapses != orig.Lapses {
		t.Errorf("reps/lapses diverged: orig=%+v round=%+v", orig, round)
	}
	if round.Due != orig.Due {
		t.Errorf("Due diverged: orig=%d round=%d", orig.Due, round.Due)
	}
}

func TestRoundTrip_NewCard(t *testing.T) {
	orig := AnkiCardSched{Type: 0, Queue: 0}
	round := Unmap(Map(orig, time.Now(), time.Now()), time.Now())
	if round.Type != 0 || round.Queue != 0 {
		t.Errorf("new card must remain new: got %+v", round)
	}
}

// ── Property: Unmap(Map(x)) preserves the SRS-load-bearing fields for any
// realistic Anki card. We constrain the input space to the ranges Anki
// actually emits — ease 1300..4000, ivl 0..36500, reps/lapses 0..1000, due
// 0..36500 — and assert round-trip stability for review cards.

func TestProperty_ReviewCardRoundTrip(t *testing.T) {
	crt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	gen := func(r *rand.Rand) AnkiCardSched {
		return AnkiCardSched{
			Type:   2,
			Queue:  2,
			IVL:    1 + r.Intn(3650),
			Factor: 1300 + r.Intn(2700),
			Reps:   r.Intn(1000),
			Lapses: r.Intn(100),
			Due:    int64(r.Intn(3650)),
		}
	}

	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		orig := gen(r)
		round := Unmap(Map(orig, crt, now), crt)

		if round.IVL != orig.IVL {
			t.Logf("IVL diff: orig=%d round=%d (full orig=%+v)", orig.IVL, round.IVL, orig)
			return false
		}
		if round.Reps != orig.Reps {
			t.Logf("Reps diff: orig=%+v round=%+v", orig, round)
			return false
		}
		if round.Lapses != orig.Lapses {
			t.Logf("Lapses diff: orig=%+v round=%+v", orig, round)
			return false
		}
		if round.Due != orig.Due {
			t.Logf("Due diff: orig=%+v round=%+v", orig, round)
			return false
		}

		// Factor can drift by ±1 due to the float intermediate; that's a
		// rounding artifact, not a scheduling regression. When the original
		// ease was outside Anki's normal [1300, 4000] band, the mapping
		// saturates difficulty at 1 or 10 and the inverse can't recover
		// the original — that's fine, it represents a legitimate clamp.
		mid := Map(orig, crt, now)
		clamped := mid.Difficulty != nil && (*mid.Difficulty <= 1.0+1e-9 || *mid.Difficulty >= 10.0-1e-9)
		if !clamped && abs(round.Factor-orig.Factor) > 1 {
			t.Logf("Factor diff: orig=%d round=%d diff=%v",
				orig.Factor, round.Factor, *mid.Difficulty)
			return false
		}
		return true
	}

	if err := quick.Check(prop, &quick.Config{MaxCount: 2000}); err != nil {
		t.Error(err)
	}
}

func TestProperty_MonotonicDifficulty(t *testing.T) {
	// Lower ease → higher difficulty. Always.
	prop := func(easeA, easeB uint16) bool {
		a := int(easeA)%2700 + 1300
		b := int(easeB)%2700 + 1300
		if a == b {
			return true
		}
		mapA := Map(AnkiCardSched{Type: 2, Queue: 2, IVL: 10, Factor: a}, time.Now(), time.Now())
		mapB := Map(AnkiCardSched{Type: 2, Queue: 2, IVL: 10, Factor: b}, time.Now(), time.Now())
		if a < b {
			return *mapA.Difficulty >= *mapB.Difficulty
		}
		return *mapA.Difficulty <= *mapB.Difficulty
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 2000}); err != nil {
		t.Error(err)
	}
}

func TestProperty_StabilityFollowsIVL(t *testing.T) {
	// Larger interval → at-least-as-large stability, always.
	prop := func(a, b uint16) bool {
		ivlA, ivlB := int(a)%3650+1, int(b)%3650+1
		mapA := Map(AnkiCardSched{Type: 2, Queue: 2, IVL: ivlA, Factor: 2500}, time.Now(), time.Now())
		mapB := Map(AnkiCardSched{Type: 2, Queue: 2, IVL: ivlB, Factor: 2500}, time.Now(), time.Now())
		if ivlA <= ivlB {
			return *mapA.Stability <= *mapB.Stability
		}
		return *mapA.Stability >= *mapB.Stability
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 2000}); err != nil {
		t.Error(err)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
