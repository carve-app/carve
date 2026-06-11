// Package fsrs implements the FSRS-6 spaced-repetition algorithm.
//
// Reference: https://github.com/open-spaced-repetition/fsrs6
// License:   MIT (FSRS algorithm is open-source)
package fsrs

import (
	"math"
	"time"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	Decay  = -0.5
	Factor = 19.0 / 81.0 // 0.9^(1/Decay) - 1

	// Default leech threshold: suspend after this many lapses.
	DefaultLeechThreshold = 5

	// Learning step durations.
	stepAgain      = 1 * time.Minute
	stepHard       = 5 * time.Minute
	stepRelearning = 10 * time.Minute
)

// State represents a card's current SRS state.
type State string

const (
	StateNew        State = "new"
	StateLearning   State = "learning"
	StateReview     State = "review"
	StateRelearning State = "relearning"
)

// Rating is the user's response quality (1–4).
type Rating int

const (
	Again Rating = 1
	Hard  Rating = 2
	Good  Rating = 3
	Easy  Rating = 4
)

// Params holds the 19 FSRS-6 weight parameters and per-user settings.
type Params struct {
	W               [19]float64
	TargetRetention float64 // default 0.90
}

// DefaultParams returns the published FSRS-6 default parameters.
func DefaultParams() Params {
	return Params{
		W: [19]float64{
			0.4072,  // w[0]  initial stability for Again
			1.1829,  // w[1]  initial stability for Hard
			3.1262,  // w[2]  initial stability for Good
			15.4722, // w[3]  initial stability for Easy
			7.2102,  // w[4]  initial difficulty (Good baseline)
			0.5316,  // w[5]  difficulty decay
			1.0651,  // w[6]  difficulty mean-reversion weight
			0.1544,  // w[7]  difficulty delta per rating
			1.0648,  // w[8]  stability growth coefficient
			0.1400,  // w[9]  stability decay exponent
			0.9988,  // w[10] retrievability effect on stability growth
			1.9813,  // w[11] post-lapse stability coefficient
			0.1100,  // w[12] difficulty effect on post-lapse stability
			0.2900,  // w[13] stability effect on post-lapse stability
			2.2700,  // w[14] retrievability effect on post-lapse stability
			0.1000,  // w[15] Hard modifier
			2.9898,  // w[16] Easy modifier
			0.5100,  // w[17] short-term stability growth rate
			0.4300,  // w[18] short-term stability offset
		},
		TargetRetention: 0.90,
	}
}

// CardState holds the mutable FSRS fields for a card.
type CardState struct {
	State       State
	Stability   float64   // S: days until 90% retention
	Difficulty  float64   // D: [1, 10]
	Due         time.Time // next review time
	LastReview  time.Time
	Reps        int
	Lapses      int
}

// ReviewResult is what the algorithm returns after processing one review.
type ReviewResult struct {
	State      State
	Stability  float64
	Difficulty float64
	Due        time.Time
	Reps       int
	Lapses     int
	// Retrievability at the time of review (0–1); 0 for new cards.
	Retrievability float64
	// IsLeech is true when Lapses >= DefaultLeechThreshold after this review.
	IsLeech bool
}

// IntervalPreview contains the predicted next interval for all 4 ratings.
type IntervalPreview struct {
	AgainDue time.Time
	HardDue  time.Time
	GoodDue  time.Time
	EasyDue  time.Time
}

// ── Core algorithm ────────────────────────────────────────────────────────────

// Retrievability returns R(t, S) — the probability of recall at elapsed time t.
func Retrievability(elapsed time.Duration, stability float64) float64 {
	if stability <= 0 {
		return 0
	}
	t := elapsed.Hours() / 24 // elapsed in days
	return math.Pow(1+Factor*t/stability, Decay)
}

// MaxIntervalDays caps a scheduled interval at ~100 years. This matches the
// FSRS reference stability cap and, critically, keeps the
// `time.Duration(days) * 24 * time.Hour` nanosecond math far below the int64
// overflow point (~106752 days), which would otherwise wrap a Due timestamp
// into the past for mature cards or crafted Anki imports.
const MaxIntervalDays = 36500

// MaxStability caps stored stability at the same horizon so the retrievability
// math never operates on an unbounded S.
const MaxStability = 36500.0

// intervalDays returns the number of days that achieves targetRetention.
// Minimum 1 day.
func intervalDays(stability, targetRetention float64) int {
	// t = S/Factor * (r^(1/Decay) - 1)
	t := stability / Factor * (math.Pow(targetRetention, 1.0/Decay) - 1)
	d := int(math.Round(t))
	if d < 1 {
		d = 1
	}
	return d
}

// clampIntervalDays bounds a final interval to [1, MaxIntervalDays]. Applied at
// each Schedule call site (after any per-rating multiplier) so the resulting
// time.Duration can never overflow.
func clampIntervalDays(days int) int {
	if days < 1 {
		return 1
	}
	if days > MaxIntervalDays {
		return MaxIntervalDays
	}
	return days
}

// initialStability returns S_0 for the first review of a new card.
func initialStability(p Params, g Rating) float64 {
	return p.W[g-1]
}

// initialDifficulty returns D_0 for a given first-review rating.
func initialDifficulty(p Params, g Rating) float64 {
	d := p.W[4] - math.Exp(p.W[5]*float64(g-1)) + 1
	return clamp(d, 1, 10)
}

// nextDifficulty applies the mean-reversion difficulty update.
func nextDifficulty(p Params, d float64, g Rating) float64 {
	d0Good := p.W[4] - math.Exp(p.W[5]*float64(Good-1)) + 1
	next := p.W[6]*d0Good + (1-p.W[6])*(d-p.W[7]*float64(g-3))
	return clamp(next, 1, 10)
}

// stabilityAfterRecall computes S'_r (card was remembered, G in {2,3,4}).
func stabilityAfterRecall(p Params, d, s, r float64, g Rating) float64 {
	var mod float64
	switch g {
	case Hard:
		mod = p.W[15]
	case Easy:
		mod = p.W[16]
	default: // Good
		mod = 1.0
	}
	sr := s * (math.Exp(p.W[8])*(11-d)*math.Pow(s, -p.W[9])*(math.Exp(p.W[10]*(1-r))-1)*mod + 1)
	if sr < 0.1 {
		sr = 0.1
	}
	if sr > MaxStability {
		sr = MaxStability
	}
	return sr
}

// stabilityAfterForgetting computes S'_f (card was forgotten, G==1 from Review).
func stabilityAfterForgetting(p Params, d, s, r float64) float64 {
	sf := p.W[11] * math.Pow(d, -p.W[12]) * (math.Pow(s+1, p.W[13]) - 1) * math.Exp(p.W[14]*(1-r))
	if sf < 0.1 {
		sf = 0.1
	}
	if sf > MaxStability {
		sf = MaxStability
	}
	return sf
}

// stabilityShortTerm computes S'_s for within-day (learning/relearning) reviews.
func stabilityShortTerm(p Params, s float64, g Rating) float64 {
	ss := s * math.Exp(p.W[17]*(float64(g)-3+p.W[18]))
	if ss < 0.1 {
		ss = 0.1
	}
	return ss
}

// ── State machine ─────────────────────────────────────────────────────────────

// Schedule processes a review event and returns the updated card state.
func Schedule(p Params, card CardState, g Rating, now time.Time) ReviewResult {
	res := ReviewResult{
		State:      card.State,
		Stability:  card.Stability,
		Difficulty: card.Difficulty,
		Reps:       card.Reps + 1,
		Lapses:     card.Lapses,
	}

	var elapsed time.Duration
	if !card.LastReview.IsZero() {
		elapsed = now.Sub(card.LastReview)
	}

	switch card.State {
	case StateNew:
		res.Stability = initialStability(p, g)
		res.Difficulty = initialDifficulty(p, g)
		res.Retrievability = 0
		switch g {
		case Again:
			res.State = StateLearning
			res.Due = now.Add(stepAgain)
		case Hard:
			res.State = StateLearning
			res.Due = now.Add(stepHard)
		case Good, Easy:
			res.State = StateReview
			days := intervalDays(res.Stability, p.TargetRetention)
			res.Due = now.Add(time.Duration(clampIntervalDays(days)) * 24 * time.Hour)
		}

	case StateLearning:
		res.Retrievability = 0
		newS := stabilityShortTerm(p, card.Stability, g)
		res.Stability = newS
		res.Difficulty = nextDifficulty(p, card.Difficulty, g)
		switch g {
		case Again:
			res.State = StateLearning
			res.Due = now.Add(stepAgain)
		case Hard:
			res.State = StateLearning
			res.Due = now.Add(stepHard)
		case Good, Easy:
			res.State = StateReview
			days := intervalDays(newS, p.TargetRetention)
			res.Due = now.Add(time.Duration(clampIntervalDays(days)) * 24 * time.Hour)
		}

	case StateReview:
		r := Retrievability(elapsed, card.Stability)
		res.Retrievability = r
		switch g {
		case Again:
			res.Lapses = card.Lapses + 1
			res.State = StateRelearning
			res.Stability = stabilityAfterForgetting(p, card.Difficulty, card.Stability, r)
			res.Difficulty = nextDifficulty(p, card.Difficulty, Again)
			res.Due = now.Add(stepRelearning)
			res.IsLeech = res.Lapses >= DefaultLeechThreshold
		default:
			newS := stabilityAfterRecall(p, card.Difficulty, card.Stability, r, g)
			res.Stability = newS
			res.Difficulty = nextDifficulty(p, card.Difficulty, g)
			res.State = StateReview
			days := intervalDays(newS, p.TargetRetention)
			res.Due = now.Add(time.Duration(clampIntervalDays(days)) * 24 * time.Hour)
		}

	case StateRelearning:
		res.Retrievability = Retrievability(elapsed, card.Stability)
		newS := stabilityShortTerm(p, card.Stability, g)
		res.Stability = newS
		res.Difficulty = nextDifficulty(p, card.Difficulty, g)
		switch g {
		case Again:
			res.State = StateRelearning
			res.Due = now.Add(stepRelearning)
		case Hard:
			res.State = StateRelearning
			res.Due = now.Add(stepHard)
		case Good, Easy:
			res.State = StateReview
			days := intervalDays(newS, p.TargetRetention)
			res.Due = now.Add(time.Duration(clampIntervalDays(days)) * 24 * time.Hour)
		}
	}

	return res
}

// Preview returns the predicted next-due time for all 4 ratings without
// mutating any state. Used to show interval hints on the review UI.
func Preview(p Params, card CardState, now time.Time) IntervalPreview {
	var pv IntervalPreview
	for _, g := range []Rating{Again, Hard, Good, Easy} {
		res := Schedule(p, card, g, now)
		switch g {
		case Again:
			pv.AgainDue = res.Due
		case Hard:
			pv.HardDue = res.Due
		case Good:
			pv.GoodDue = res.Due
		case Easy:
			pv.EasyDue = res.Due
		}
	}
	return pv
}

// WorkloadForecast returns how many cards will be due on each of the next
// `days` days given the provided due-date list.
func WorkloadForecast(dues []time.Time, days int, now time.Time) []int {
	counts := make([]int, days)
	start := now.Truncate(24 * time.Hour)
	for _, due := range dues {
		offset := int(due.Truncate(24 * time.Hour).Sub(start).Hours() / 24)
		if offset >= 0 && offset < days {
			counts[offset]++
		}
	}
	return counts
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
