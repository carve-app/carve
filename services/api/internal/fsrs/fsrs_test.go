package fsrs

import (
	"math"
	"testing"
	"time"
)

var p = DefaultParams()
var now = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// ── Helpers ───────────────────────────────────────────────────────────────────

func newCard() CardState {
	return CardState{State: StateNew}
}

func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// ── Retrievability ────────────────────────────────────────────────────────────

func TestRetrievabilityAtZero(t *testing.T) {
	r := Retrievability(0, 10)
	if r != 1.0 {
		t.Errorf("R(0,S) should be 1.0, got %v", r)
	}
}

func TestRetrievabilityAt90Pct(t *testing.T) {
	// When elapsed == stability, R should be ~0.9 (by FSRS definition)
	s := 10.0
	elapsed := time.Duration(s*24) * time.Hour
	r := Retrievability(elapsed, s)
	if !approxEqual(r, 0.9, 0.001) {
		t.Errorf("R(S,S) should be ~0.9, got %v", r)
	}
}

func TestRetrievabilityDecays(t *testing.T) {
	s := 10.0
	r1 := Retrievability(5*24*time.Hour, s)
	r2 := Retrievability(10*24*time.Hour, s)
	r3 := Retrievability(20*24*time.Hour, s)
	if !(r1 > r2 && r2 > r3) {
		t.Errorf("retrievability should decay monotonically: %v > %v > %v", r1, r2, r3)
	}
}

func TestRetrievabilityZeroStability(t *testing.T) {
	r := Retrievability(time.Hour, 0)
	if r != 0 {
		t.Errorf("R with zero stability should be 0, got %v", r)
	}
}

// ── New card transitions ──────────────────────────────────────────────────────

func TestNewCardAgain(t *testing.T) {
	res := Schedule(p, newCard(), Again, now)
	if res.State != StateLearning {
		t.Errorf("Again from New → Learning, got %v", res.State)
	}
	if res.Due.Sub(now) != stepAgain {
		t.Errorf("Again step should be %v, got %v", stepAgain, res.Due.Sub(now))
	}
	if res.Stability != p.W[0] {
		t.Errorf("initial stability for Again should be w[0]=%v, got %v", p.W[0], res.Stability)
	}
}

func TestNewCardHard(t *testing.T) {
	res := Schedule(p, newCard(), Hard, now)
	if res.State != StateLearning {
		t.Errorf("Hard from New → Learning, got %v", res.State)
	}
	if res.Due.Sub(now) != stepHard {
		t.Errorf("Hard step should be %v, got %v", stepHard, res.Due.Sub(now))
	}
	if res.Stability != p.W[1] {
		t.Errorf("initial stability for Hard should be w[1]=%v, got %v", p.W[1], res.Stability)
	}
}

func TestNewCardGood(t *testing.T) {
	res := Schedule(p, newCard(), Good, now)
	if res.State != StateReview {
		t.Errorf("Good from New → Review, got %v", res.State)
	}
	days := int(res.Due.Sub(now).Hours() / 24)
	if days < 1 {
		t.Errorf("Good interval should be ≥ 1 day, got %v", days)
	}
	if res.Stability != p.W[2] {
		t.Errorf("initial stability for Good should be w[2]=%v, got %v", p.W[2], res.Stability)
	}
}

func TestNewCardEasy(t *testing.T) {
	res := Schedule(p, newCard(), Easy, now)
	if res.State != StateReview {
		t.Errorf("Easy from New → Review, got %v", res.State)
	}
	goodRes := Schedule(p, newCard(), Good, now)
	easyDays := int(res.Due.Sub(now).Hours() / 24)
	goodDays := int(goodRes.Due.Sub(now).Hours() / 24)
	if easyDays < goodDays {
		t.Errorf("Easy interval (%v) should be >= Good interval (%v)", easyDays, goodDays)
	}
}

// TestNewCardIntervalDays pins the actual scheduled day counts (not just the
// Easy>=Good ordering) so a regression that re-introduces a spurious interval
// multiplier — like the old w[16] bug that ~3x-inflated Easy intervals — is
// caught. At TargetRetention=0.90 the interval equals round(initialStability):
// Good → round(w[2]=3.1262)=3, Easy → round(w[3]=15.4722)=15.
func TestNewCardIntervalDays(t *testing.T) {
	goodDays := int(Schedule(p, newCard(), Good, now).Due.Sub(now).Hours() / 24)
	easyDays := int(Schedule(p, newCard(), Easy, now).Due.Sub(now).Hours() / 24)
	if goodDays != 3 {
		t.Errorf("new Good interval = %d days, want 3", goodDays)
	}
	if easyDays != 15 {
		t.Errorf("new Easy interval = %d days, want 15", easyDays)
	}
}

// TestIntervalDaysClamped verifies the interval clamp prevents the int64
// nanosecond overflow that previously corrupted Due for very large stability.
func TestIntervalDaysClamped(t *testing.T) {
	card := CardState{
		State:      StateReview,
		Stability:  1e9, // absurdly large; round(S) would dwarf the overflow point
		Difficulty: 5,
		Reps:       100,
		LastReview: now.Add(-24 * time.Hour),
	}
	res := Schedule(p, card, Good, now)
	gotDays := res.Due.Sub(now).Hours() / 24
	if gotDays <= 0 {
		t.Fatalf("Due overflowed into the past: %v days", gotDays)
	}
	if int(gotDays+0.5) > MaxIntervalDays {
		t.Errorf("interval %v days exceeds MaxIntervalDays %d", gotDays, MaxIntervalDays)
	}
}

// ── Difficulty initialization ─────────────────────────────────────────────────

func TestDifficultyRange(t *testing.T) {
	for _, g := range []Rating{Again, Hard, Good, Easy} {
		res := Schedule(p, newCard(), g, now)
		if res.Difficulty < 1 || res.Difficulty > 10 {
			t.Errorf("difficulty for rating %v out of range [1,10]: %v", g, res.Difficulty)
		}
	}
}

func TestDifficultyOrderedByRating(t *testing.T) {
	dAgain := Schedule(p, newCard(), Again, now).Difficulty
	dEasy := Schedule(p, newCard(), Easy, now).Difficulty
	if dAgain <= dEasy {
		t.Errorf("Again should produce higher difficulty than Easy: %v vs %v", dAgain, dEasy)
	}
}

// ── Learning state transitions ────────────────────────────────────────────────

func TestLearningAgainStaysLearning(t *testing.T) {
	c := CardState{
		State:      StateLearning,
		Stability:  p.W[0],
		Difficulty: 5.0,
		LastReview: now.Add(-2 * time.Minute),
	}
	res := Schedule(p, c, Again, now)
	if res.State != StateLearning {
		t.Errorf("Again from Learning → Learning, got %v", res.State)
	}
}

func TestLearningGoodGraduates(t *testing.T) {
	c := CardState{
		State:      StateLearning,
		Stability:  p.W[2],
		Difficulty: 5.0,
		LastReview: now.Add(-5 * time.Minute),
	}
	res := Schedule(p, c, Good, now)
	if res.State != StateReview {
		t.Errorf("Good from Learning → Review, got %v", res.State)
	}
	days := int(res.Due.Sub(now).Hours() / 24)
	if days < 1 {
		t.Errorf("graduated interval should be ≥ 1 day, got %v", days)
	}
}

// ── Review state transitions ──────────────────────────────────────────────────

func TestReviewAgainLapses(t *testing.T) {
	c := CardState{
		State:      StateReview,
		Stability:  10.0,
		Difficulty: 5.0,
		LastReview: now.Add(-15 * 24 * time.Hour),
		Lapses:     0,
	}
	res := Schedule(p, c, Again, now)
	if res.State != StateRelearning {
		t.Errorf("Again from Review → Relearning, got %v", res.State)
	}
	if res.Lapses != 1 {
		t.Errorf("Lapses should increment to 1, got %v", res.Lapses)
	}
	if res.Due.Sub(now) != stepRelearning {
		t.Errorf("relearning step should be %v, got %v", stepRelearning, res.Due.Sub(now))
	}
}

func TestReviewGoodIncreasesStability(t *testing.T) {
	c := CardState{
		State:      StateReview,
		Stability:  10.0,
		Difficulty: 5.0,
		LastReview: now.Add(-10 * 24 * time.Hour),
	}
	res := Schedule(p, c, Good, now)
	if res.Stability <= c.Stability {
		t.Errorf("stability after Good review should increase: %v → %v", c.Stability, res.Stability)
	}
}

func TestReviewEasyHigherThanGood(t *testing.T) {
	c := CardState{
		State:      StateReview,
		Stability:  10.0,
		Difficulty: 5.0,
		LastReview: now.Add(-10 * 24 * time.Hour),
	}
	goodRes := Schedule(p, c, Good, now)
	easyRes := Schedule(p, c, Easy, now)
	if easyRes.Due.Before(goodRes.Due) {
		t.Errorf("Easy should schedule farther than Good")
	}
}

func TestReviewHardLowerThanGood(t *testing.T) {
	c := CardState{
		State:      StateReview,
		Stability:  10.0,
		Difficulty: 5.0,
		LastReview: now.Add(-10 * 24 * time.Hour),
	}
	hardRes := Schedule(p, c, Hard, now)
	goodRes := Schedule(p, c, Good, now)
	if hardRes.Due.After(goodRes.Due) {
		t.Errorf("Hard should schedule sooner than Good")
	}
}

// ── Leech detection ───────────────────────────────────────────────────────────

func TestLeechTriggeredAtThreshold(t *testing.T) {
	c := CardState{
		State:      StateReview,
		Stability:  5.0,
		Difficulty: 8.0,
		LastReview: now.Add(-7 * 24 * time.Hour),
		Lapses:     DefaultLeechThreshold - 1,
	}
	res := Schedule(p, c, Again, now)
	if !res.IsLeech {
		t.Errorf("should be leech at %v lapses, IsLeech=%v", res.Lapses, res.IsLeech)
	}
}

func TestLeechNotTriggeredBelowThreshold(t *testing.T) {
	c := CardState{
		State:      StateReview,
		Stability:  5.0,
		Difficulty: 8.0,
		LastReview: now.Add(-7 * 24 * time.Hour),
		Lapses:     DefaultLeechThreshold - 2,
	}
	res := Schedule(p, c, Again, now)
	if res.IsLeech {
		t.Errorf("should not be leech at %v lapses", res.Lapses)
	}
}

// ── Relearning state ──────────────────────────────────────────────────────────

func TestRelearningGoodGraduates(t *testing.T) {
	c := CardState{
		State:      StateRelearning,
		Stability:  2.0,
		Difficulty: 7.0,
		LastReview: now.Add(-5 * time.Minute),
		Lapses:     1,
	}
	res := Schedule(p, c, Good, now)
	if res.State != StateReview {
		t.Errorf("Good from Relearning → Review, got %v", res.State)
	}
}

// ── Preview ───────────────────────────────────────────────────────────────────

func TestPreviewOrderedCorrectly(t *testing.T) {
	c := CardState{
		State:      StateReview,
		Stability:  10.0,
		Difficulty: 5.0,
		LastReview: now.Add(-10 * 24 * time.Hour),
	}
	pv := Preview(p, c, now)
	if !pv.AgainDue.Before(pv.HardDue) {
		t.Errorf("Again due (%v) should be before Hard due (%v)", pv.AgainDue, pv.HardDue)
	}
	if !pv.HardDue.Before(pv.GoodDue) {
		t.Errorf("Hard due (%v) should be before Good due (%v)", pv.HardDue, pv.GoodDue)
	}
	if !pv.GoodDue.Before(pv.EasyDue) {
		t.Errorf("Good due (%v) should be before Easy due (%v)", pv.GoodDue, pv.EasyDue)
	}
}

// ── WorkloadForecast ──────────────────────────────────────────────────────────

func TestWorkloadForecast(t *testing.T) {
	dues := []time.Time{
		now.Add(0 * 24 * time.Hour),  // today
		now.Add(0 * 24 * time.Hour),  // today
		now.Add(1 * 24 * time.Hour),  // tomorrow
		now.Add(7 * 24 * time.Hour),  // day 7
		now.Add(15 * 24 * time.Hour), // beyond window
	}
	counts := WorkloadForecast(dues, 14, now)
	if len(counts) != 14 {
		t.Errorf("expected 14-day forecast, got %v days", len(counts))
	}
	if counts[0] != 2 {
		t.Errorf("today should have 2 cards, got %v", counts[0])
	}
	if counts[1] != 1 {
		t.Errorf("tomorrow should have 1 card, got %v", counts[1])
	}
	if counts[7] != 1 {
		t.Errorf("day 7 should have 1 card, got %v", counts[7])
	}
}

// ── Reps counter ──────────────────────────────────────────────────────────────

func TestRepsIncrement(t *testing.T) {
	c := newCard()
	res := Schedule(p, c, Good, now)
	if res.Reps != 1 {
		t.Errorf("reps should be 1 after first review, got %v", res.Reps)
	}
	c2 := CardState{
		State:      res.State,
		Stability:  res.Stability,
		Difficulty: res.Difficulty,
		Due:        res.Due,
		LastReview: now,
		Reps:       res.Reps,
	}
	res2 := Schedule(p, c2, Good, now.Add(3*24*time.Hour))
	if res2.Reps != 2 {
		t.Errorf("reps should be 2 after second review, got %v", res2.Reps)
	}
}

// ── Stability sanity checks ────────────────────────────────────────────────────

func TestStabilityPositive(t *testing.T) {
	states := []State{StateNew, StateLearning, StateReview, StateRelearning}
	ratings := []Rating{Again, Hard, Good, Easy}
	for _, st := range states {
		for _, g := range ratings {
			c := CardState{
				State:      st,
				Stability:  5.0,
				Difficulty: 5.0,
				LastReview: now.Add(-5 * 24 * time.Hour),
			}
			res := Schedule(p, c, g, now)
			if res.Stability <= 0 {
				t.Errorf("stability should be positive for state=%v rating=%v, got %v", st, g, res.Stability)
			}
		}
	}
}

// ── intervalDays ─────────────────────────────────────────────────────────────

func TestIntervalDaysAt90Pct(t *testing.T) {
	// For target=0.9, interval should equal stability
	s := 10.0
	d := intervalDays(s, 0.90)
	if d != 10 {
		t.Errorf("interval at 90%% retention with S=10 should be 10, got %v", d)
	}
}

func TestIntervalDaysMinimumOne(t *testing.T) {
	d := intervalDays(0.1, 0.9)
	if d < 1 {
		t.Errorf("interval should be at least 1, got %v", d)
	}
}
