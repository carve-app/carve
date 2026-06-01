package fsrs

import (
	"math/rand"
	"testing"
	"testing/quick"
	"time"
)

// ── Properties the FSRS scheduler must satisfy ──────────────────────────────
//
// These are correctness invariants every implementation owes the user.
// quick.Check throws random inputs at each property; failures shrink to
// the seed that broke them so we can write a deterministic regression.

func newCardState() CardState {
	return CardState{
		State:      StateNew,
		Stability:  0,
		Difficulty: 5,
		Reps:       0,
		Lapses:     0,
	}
}

// Easy outcomes should always schedule farther in the future than Again.
func TestProperty_EasyDelaysMoreThanAgain(t *testing.T) {
	p := DefaultParams()
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		cs := CardState{
			State:      StateReview,
			Stability:  1 + r.Float64()*200,
			Difficulty: 1 + r.Float64()*9,
			Reps:       r.Intn(50),
			Lapses:     r.Intn(20),
			LastReview: time.Now().Add(-time.Duration(r.Intn(100)) * 24 * time.Hour),
		}
		now := time.Now()
		again := Schedule(p, cs, Again, now)
		easy := Schedule(p, cs, Easy, now)
		return easy.Due.After(again.Due)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 1000}); err != nil {
		t.Error(err)
	}
}

// Stability after a successful review (Good/Easy) never decreases.
func TestProperty_StabilityNonDecreasingOnPass(t *testing.T) {
	p := DefaultParams()
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		cs := CardState{
			State:      StateReview,
			Stability:  1 + r.Float64()*200,
			Difficulty: 1 + r.Float64()*9,
			Reps:       r.Intn(50),
			Lapses:     r.Intn(20),
			LastReview: time.Now().Add(-time.Duration(r.Intn(100)) * 24 * time.Hour),
		}
		now := time.Now()
		good := Schedule(p, cs, Good, now)
		easy := Schedule(p, cs, Easy, now)
		return good.Stability >= cs.Stability && easy.Stability >= cs.Stability
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 1000}); err != nil {
		t.Error(err)
	}
}

// "Again" rating must always increment Lapses for review-state cards.
func TestProperty_AgainBumpsLapses(t *testing.T) {
	p := DefaultParams()
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		cs := CardState{
			State:      StateReview,
			Stability:  1 + r.Float64()*200,
			Difficulty: 1 + r.Float64()*9,
			Reps:       1 + r.Intn(50),
			Lapses:     r.Intn(20),
			LastReview: time.Now().Add(-time.Duration(r.Intn(100)) * 24 * time.Hour),
		}
		out := Schedule(p, cs, Again, time.Now())
		return out.Lapses == cs.Lapses+1
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Error(err)
	}
}

// Reps must strictly increase on every rating.
func TestProperty_RepsAlwaysIncrement(t *testing.T) {
	p := DefaultParams()
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		cs := CardState{
			State:      StateReview,
			Stability:  1 + r.Float64()*200,
			Difficulty: 1 + r.Float64()*9,
			Reps:       r.Intn(50),
			Lapses:     r.Intn(20),
			LastReview: time.Now().Add(-time.Duration(r.Intn(100)) * 24 * time.Hour),
		}
		now := time.Now()
		for _, rating := range []Rating{Again, Hard, Good, Easy} {
			out := Schedule(p, cs, rating, now)
			if out.Reps != cs.Reps+1 {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Error(err)
	}
}

// Difficulty stays within [1, 10] regardless of input.
func TestProperty_DifficultyClampedTo1_10(t *testing.T) {
	p := DefaultParams()
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		cs := CardState{
			State:      StateReview,
			Stability:  1 + r.Float64()*200,
			Difficulty: 1 + r.Float64()*9,
			Reps:       r.Intn(50),
			LastReview: time.Now().Add(-time.Duration(r.Intn(100)) * 24 * time.Hour),
		}
		now := time.Now()
		for _, rating := range []Rating{Again, Hard, Good, Easy} {
			out := Schedule(p, cs, rating, now)
			if out.Difficulty < 1 || out.Difficulty > 10 {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 1000}); err != nil {
		t.Error(err)
	}
}
