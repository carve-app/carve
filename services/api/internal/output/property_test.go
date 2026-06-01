package output

import (
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
)

// ── Properties the diff/WER engine must satisfy ─────────────────────────────

// WER of any string against itself is 0.
func TestProperty_WERSelfIsZero(t *testing.T) {
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		s := randomSentence(r)
		_, wer := computeDiff(s, s)
		return wer == 0
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Error(err)
	}
}

// WER against the empty string is 1 for any non-empty reference (every
// reference word is a deletion).
func TestProperty_WEREmptyHypothesisIsOne(t *testing.T) {
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		s := randomSentence(r)
		if strings.TrimSpace(s) == "" {
			return true // vacuously true
		}
		_, wer := computeDiff(s, "")
		return wer == 1
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Error(err)
	}
}

// WER is bounded in [0, ∞) — never negative, never NaN.
func TestProperty_WERNonNegative(t *testing.T) {
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		ref := randomSentence(r)
		hyp := randomSentence(r)
		_, wer := computeDiff(ref, hyp)
		return wer >= 0
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Error(err)
	}
}

// Diff length equals the union of keep + sub + insert + delete edits.
// In particular, every token in ref appears as a keep/sub/delete, and every
// token in hyp appears as a keep/sub/insert.
func TestProperty_DiffCountsBalance(t *testing.T) {
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		ref := randomSentence(r)
		hyp := randomSentence(r)
		diff, _ := computeDiff(ref, hyp)
		refTokens := normalize(ref)
		hypTokens := normalize(hyp)

		var refContrib, hypContrib int
		for _, e := range diff {
			switch e.Op {
			case "keep", "sub":
				refContrib++
				hypContrib++
			case "delete":
				refContrib++
			case "insert":
				hypContrib++
			}
		}
		return refContrib == len(refTokens) && hypContrib == len(hypTokens)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Error(err)
	}
}

// Casing differences and trailing punctuation never affect WER.
func TestProperty_NormalizationInvariant(t *testing.T) {
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		ref := randomSentence(r)
		variants := []string{ref, strings.ToUpper(ref), strings.ToLower(ref), ref + ".", ref + "!", "  " + ref + "  "}
		_, base := computeDiff(ref, variants[0])
		for _, v := range variants[1:] {
			_, w := computeDiff(ref, v)
			if w != base {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Error(err)
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

var sentencePool = []string{
	"the quick brown fox jumps over the lazy dog",
	"hello world how are you today",
	"language learning is a marathon not a sprint",
	"she walked into the room with confidence",
	"please pass the salt and pepper",
	"the meeting starts at three in the afternoon",
	"we should probably go home now",
	"a journey of a thousand miles begins with a single step",
}

func randomSentence(r *rand.Rand) string {
	base := sentencePool[r.Intn(len(sentencePool))]
	words := strings.Fields(base)
	if len(words) <= 1 {
		return base
	}
	// Randomly mutate by dropping or duplicating one word
	switch r.Intn(3) {
	case 0:
		// Drop a word
		idx := r.Intn(len(words))
		words = append(words[:idx], words[idx+1:]...)
	case 1:
		// Duplicate a word
		idx := r.Intn(len(words))
		words = append(words[:idx+1], append([]string{words[idx]}, words[idx+1:]...)...)
	}
	return strings.Join(words, " ")
}
