package fsrs

import (
	"math"
	"sort"
	"time"
)

// ReviewEvent is a single review event used by the optimizer.
type ReviewEvent struct {
	CardID     string
	Rating     Rating
	ReviewedAt time.Time
}

// OptimizeResult contains the optimized parameters and diagnostics.
type OptimizeResult struct {
	Params    Params
	FinalLoss float64
	Iters     int
}

// Optimize runs Adam gradient descent over the provided review history to find
// FSRS-6 weights that minimize binary cross-entropy between the model's
// predicted retrievability and the observed recall outcomes.
//
// Events may be in any order; they will be sorted and grouped by card internally.
// Requires at least 400 review events to be reliable; returns initialParams unchanged
// if there are fewer than 100 usable review-state events.
func Optimize(initialParams Params, events []ReviewEvent, maxIter int) OptimizeResult {
	if maxIter <= 0 {
		maxIter = 100
	}

	grouped := groupByCard(events)
	usableCount := countUsableEvents(initialParams, grouped)
	if usableCount < 100 {
		return OptimizeResult{Params: initialParams, Iters: 0}
	}

	p := initialParams

	// Adam hyper-parameters.
	const (
		lr   = 4e-4
		b1   = 0.9
		b2   = 0.999
		eps  = 1e-8
		h    = 1e-4 // finite-difference step
	)

	var m, v [19]float64

	var finalLoss float64
	for iter := 1; iter <= maxIter; iter++ {
		loss := computeLoss(p, grouped)
		finalLoss = loss
		grads := numericalGrads(p, grouped, h)

		t := float64(iter)
		for i := range p.W {
			m[i] = b1*m[i] + (1-b1)*grads[i]
			v[i] = b2*v[i] + (1-b2)*grads[i]*grads[i]
			mHat := m[i] / (1 - math.Pow(b1, t))
			vHat := v[i] / (1 - math.Pow(b2, t))
			p.W[i] -= lr * mHat / (math.Sqrt(vHat) + eps)
			p.W[i] = clampWeight(i, p.W[i])
		}
	}

	return OptimizeResult{Params: p, FinalLoss: finalLoss, Iters: maxIter}
}

// groupByCard groups and sorts events by card ID + reviewed_at.
func groupByCard(events []ReviewEvent) map[string][]ReviewEvent {
	grouped := make(map[string][]ReviewEvent, len(events)/5)
	for _, e := range events {
		grouped[e.CardID] = append(grouped[e.CardID], e)
	}
	for id := range grouped {
		sort.Slice(grouped[id], func(i, j int) bool {
			return grouped[id][i].ReviewedAt.Before(grouped[id][j].ReviewedAt)
		})
	}
	return grouped
}

// computeLoss calculates mean BCE loss over all review-state recall events.
func computeLoss(p Params, grouped map[string][]ReviewEvent) float64 {
	total := 0.0
	n := 0

	for _, events := range grouped {
		card := CardState{State: StateNew}
		var lastReview time.Time

		for _, ev := range events {
			if card.State == StateReview && !lastReview.IsZero() {
				elapsed := ev.ReviewedAt.Sub(lastReview)
				days := elapsed.Hours() / 24
				if days >= 0.5 { // only score review-phase events with meaningful elapsed time
					R := Retrievability(elapsed, card.Stability)
					R = clamp(R, 1e-7, 1-1e-7)
					outcome := 0.0
					if ev.Rating >= 2 {
						outcome = 1.0
					}
					bce := -outcome*math.Log(R) - (1-outcome)*math.Log(1-R)
					total += bce
					n++
				}
			}

			result := Schedule(p, card, ev.Rating, ev.ReviewedAt)
			card = CardState{
				State:      result.State,
				Stability:  result.Stability,
				Difficulty: result.Difficulty,
				Reps:       result.Reps,
				Lapses:     result.Lapses,
			}
			lastReview = ev.ReviewedAt
		}
	}

	if n == 0 {
		return 0
	}
	return total / float64(n)
}

// numericalGrads computes finite-difference gradients for all 19 weights.
func numericalGrads(p Params, grouped map[string][]ReviewEvent, h float64) [19]float64 {
	var grads [19]float64
	for i := range p.W {
		pPlus := p
		pPlus.W[i] += h
		pMinus := p
		pMinus.W[i] -= h
		grads[i] = (computeLoss(pPlus, grouped) - computeLoss(pMinus, grouped)) / (2 * h)
	}
	return grads
}

// countUsableEvents returns the number of review-phase events (the ones
// the optimizer can learn from).
func countUsableEvents(p Params, grouped map[string][]ReviewEvent) int {
	n := 0
	for _, events := range grouped {
		card := CardState{State: StateNew}
		var lastReview time.Time
		for _, ev := range events {
			if card.State == StateReview && !lastReview.IsZero() {
				elapsed := ev.ReviewedAt.Sub(lastReview)
				if elapsed.Hours()/24 >= 0.5 {
					n++
				}
			}
			result := Schedule(p, card, ev.Rating, ev.ReviewedAt)
			card = CardState{
				State:      result.State,
				Stability:  result.Stability,
				Difficulty: result.Difficulty,
				Reps:       result.Reps,
				Lapses:     result.Lapses,
			}
			lastReview = ev.ReviewedAt
		}
	}
	return n
}

// clampWeight applies per-weight bounds derived from the FSRS-6 spec.
// Wide bounds allow meaningful optimization while preventing runaway values.
func clampWeight(i int, w float64) float64 {
	switch i {
	case 0, 1, 2, 3: // initial stabilities: must be positive
		return clamp(w, 0.1, 40.0)
	case 4: // initial difficulty base
		return clamp(w, 1.0, 12.0)
	case 5: // difficulty decay exponent
		return clamp(w, 0.01, 3.0)
	case 6: // mean-reversion weight
		return clamp(w, 0.1, 1.0)
	case 7: // difficulty delta
		return clamp(w, 0.01, 1.0)
	case 8: // recall stability coefficient
		return clamp(w, 0.1, 5.0)
	case 9: // recall stability decay
		return clamp(w, 0.01, 1.0)
	case 10: // retrievability factor in recall stability
		return clamp(w, 0.1, 5.0)
	case 11: // forgetting stability coefficient
		return clamp(w, 0.1, 10.0)
	case 12, 13, 14: // forgetting stability sub-exponents
		return clamp(w, 0.01, 5.0)
	case 15: // Hard modifier (< 1 slows growth)
		return clamp(w, 0.01, 1.0)
	case 16: // Easy modifier (> 1 boosts growth)
		return clamp(w, 1.0, 6.0)
	case 17: // short-term stability rate
		return clamp(w, 0.01, 3.0)
	case 18: // short-term stability offset
		return clamp(w, 0.01, 3.0)
	}
	return clamp(w, 0.01, 30.0)
}
