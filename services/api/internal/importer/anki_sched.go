// SM-2 → FSRS mapping for Anki imports. The goal is review-history
// preservation: a user importing a 10k-card deck must not lose years of
// scheduling progress. We map Anki's per-card state (type, ivl, factor,
// reps, lapses, due, queue) into the FSRS-6 fields the carve scheduler
// reads (fsrs_state, fsrs_stability, fsrs_difficulty, fsrs_due,
// fsrs_reps, fsrs_lapses).
//
// References:
//   - Anki cards.queue meaning:
//       0=new   1=(re)learning   2=review   3=day-learning
//      -1=suspended  -2=buried-sched  -3=buried-user
//   - Anki cards.type:
//       0=new   1=learning   2=review   3=relearning
//   - Anki cards.factor is ease × 1000 (so 2500 = 2.5).
//   - FSRS stability: time until recall probability reaches 90%, in days.
//     For migrated Anki cards we set stability = ivl (days). This is a
//     conservative under-estimate that costs at most one extra review.
//   - FSRS difficulty: [1,10]. Anki ease is in [1300, 4000+]. The mapping
//     puts the default ease (2500) at FSRS difficulty 4 and clamps either
//     side.

package importer

import (
	"math"
	"time"
)

// AnkiCardSched is the scheduling slice we read out of the .apkg cards
// table. Field names mirror the Anki schema for ease of cross-reference.
type AnkiCardSched struct {
	Type   int   // 0=new 1=learn 2=review 3=relearn
	Queue  int   // 0=new 1=learn 2=review 3=daylearn -1=suspended -2=buried-sched -3=buried-user
	IVL    int   // current interval in days (>=0)
	Factor int   // ease factor × 1000
	Reps   int   // total reviews on this card
	Lapses int   // times the card was forgotten
	Due    int64 // for review cards: days since collection epoch. for new/learn: position or unix ms.
}

// FSRSSched is the carve-side scheduling state populated by Map().
type FSRSSched struct {
	State      string     // 'new' | 'learning' | 'review' | 'relearning' | 'suspended'
	Stability  *float64   // days
	Difficulty *float64   // 1..10
	Due        *time.Time // absolute due time
	Reps       int
	Lapses     int
}

// Map converts an AnkiCardSched into a FSRSSched. `collectionCreated` is
// the .apkg's `col.crt` (collection creation time) and is used to convert
// Anki's relative-day `due` into an absolute timestamp.
func Map(a AnkiCardSched, collectionCreated time.Time, now time.Time) FSRSSched {
	out := FSRSSched{
		Reps:   a.Reps,
		Lapses: a.Lapses,
	}

	switch {
	case a.Queue == -1:
		out.State = "suspended"
	case a.Queue == -2 || a.Queue == -3:
		// Both buried states map to 'review' on next session — carve doesn't
		// model buried separately (it uses queue gating instead).
		out.State = "review"
	case a.Type == 0 || a.Queue == 0:
		out.State = "new"
		return out
	case a.Type == 1 || a.Queue == 1 || a.Queue == 3:
		out.State = "learning"
	case a.Type == 3:
		out.State = "relearning"
	default: // a.Type == 2
		out.State = "review"
	}

	// Stability (days). For review cards, use the interval. For learning
	// cards, use a small positive value so the scheduler still has a real
	// number to work with rather than NULL.
	if out.State == "review" || out.State == "suspended" || out.State == "relearning" {
		s := math.Max(1, float64(a.IVL))
		out.Stability = &s
	} else if out.State == "learning" {
		s := 0.5
		out.Stability = &s
	}

	// Difficulty: ease 2500 → 4.0; clamp range [1, 10].
	// Empirically derived: diff = 4 - (factor-2500)/300 keeps the gradient
	// reasonable across the typical Anki ease range of 1300..3500.
	if a.Factor > 0 {
		d := 4.0 - float64(a.Factor-2500)/300.0
		if d < 1 {
			d = 1
		} else if d > 10 {
			d = 10
		}
		out.Difficulty = &d
	} else {
		d := 5.0
		out.Difficulty = &d
	}

	// Due time. For review cards Anki stores days-since-`crt`. For learning
	// cards, due is seconds-since-epoch (in queue 1) or a position (queue 3),
	// which we treat as "due now" since the carve scheduler will re-anchor it.
	if out.State == "review" || out.State == "suspended" {
		t := collectionCreated.Add(time.Duration(a.Due) * 24 * time.Hour)
		out.Due = &t
	} else if out.State == "learning" || out.State == "relearning" {
		// Schedule learning cards in a few minutes — matches Anki's behavior
		// of inserting them back into today's queue after a short delay.
		t := now.Add(10 * time.Minute)
		out.Due = &t
	}

	return out
}

// Unmap is the inverse: given a FSRS state, produce an Anki-shaped record.
// Used by the round-trip property test to verify that two import/export
// cycles converge.
func Unmap(f FSRSSched, collectionCreated time.Time) AnkiCardSched {
	out := AnkiCardSched{
		Reps:   f.Reps,
		Lapses: f.Lapses,
	}

	switch f.State {
	case "new":
		out.Type = 0
		out.Queue = 0
	case "learning":
		out.Type = 1
		out.Queue = 1
	case "relearning":
		out.Type = 3
		out.Queue = 1
	case "suspended":
		out.Type = 2
		out.Queue = -1
	default: // "review"
		out.Type = 2
		out.Queue = 2
	}

	if f.Stability != nil {
		out.IVL = int(math.Round(*f.Stability))
		if out.IVL < 1 && (out.Type == 2) {
			out.IVL = 1
		}
	}

	if f.Difficulty != nil {
		// invert the linear mapping
		out.Factor = int(math.Round((4.0-*f.Difficulty)*300.0 + 2500.0))
		if out.Factor < 1300 {
			out.Factor = 1300
		}
	} else {
		out.Factor = 2500
	}

	if f.Due != nil && (out.Type == 2) {
		days := int(f.Due.Sub(collectionCreated).Hours() / 24)
		out.Due = int64(days)
	}

	return out
}
