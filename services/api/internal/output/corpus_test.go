// L12 — synthetic STT corpus. Golden fixture pairs that exercise the
// diff/WER pipeline against well-understood inputs. When the diff
// algorithm or normalization rules change, this is the first test that
// will tell you whether the change is intentional.
//
// The corpus deliberately exercises:
//   - perfect match (WER 0)
//   - single substitution
//   - single insertion
//   - single deletion
//   - leading/trailing whitespace
//   - punctuation differences
//   - capitalization differences
//   - empty hypothesis (WER 1.0)
//   - longer paragraphs with multiple edits
//   - filler words (uh, um) that should still diff cleanly

package output

import (
	"math"
	"testing"
)

type corpusCase struct {
	name        string
	reference   string
	hypothesis  string
	expectedWER float64
	tolerance   float64
}

var corpus = []corpusCase{
	{"perfect_match", "the cat sat on the mat", "the cat sat on the mat", 0.0, 0.0},
	{"single_substitution", "the cat sat on the mat", "the cat sat on the rug", 1.0 / 6, 0.001},
	{"single_insertion", "the cat sat on the mat", "the cat actually sat on the mat", 1.0 / 6, 0.001},
	{"single_deletion", "the cat sat on the mat", "the cat sat on the", 1.0 / 6, 0.001},
	{"whitespace_invariant", "  the cat sat  ", "the cat sat", 0.0, 0.0},
	{"punctuation_invariant", "Hello, world!", "hello world", 0.0, 0.0},
	{"case_invariant", "Hello World", "hello world", 0.0, 0.0},
	{"empty_hypothesis", "the cat sat", "", 1.0, 0.0},
	{"filler_words_added", "I went to the store", "I uh went to the store", 1.0 / 5, 0.001},
	{
		name:        "long_paragraph_with_two_edits",
		reference:   "language learning is a marathon not a sprint and immersion matters",
		hypothesis:  "language learning is a journey not a sprint but immersion matters",
		expectedWER: 2.0 / 11,
		tolerance:   0.001,
	},
}

func TestCorpus_DiffAndWERMatchGolden(t *testing.T) {
	for _, c := range corpus {
		t.Run(c.name, func(t *testing.T) {
			_, wer := computeDiff(c.reference, c.hypothesis)
			if math.Abs(wer-c.expectedWER) > c.tolerance {
				t.Errorf("WER mismatch: ref=%q hyp=%q got=%.4f want=%.4f",
					c.reference, c.hypothesis, wer, c.expectedWER)
			}
		})
	}
}

func TestCorpus_DiffOpClassification(t *testing.T) {
	// For each golden case, verify the diff classifies each edit correctly.
	cases := []struct {
		name         string
		ref, hyp     string
		hasSub       bool
		hasInsert    bool
		hasDelete    bool
	}{
		{"sub_only",    "the cat sat", "the dog sat",       true,  false, false},
		{"insert_only", "the cat sat", "the big cat sat",   false, true,  false},
		{"delete_only", "the big cat", "the cat",           false, false, true},
		{"mixed",       "a b c d",     "a x c",             true,  false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diff, _ := computeDiff(c.ref, c.hyp)
			var hasSub, hasIns, hasDel bool
			for _, e := range diff {
				switch e.Op {
				case "sub":    hasSub = true
				case "insert": hasIns = true
				case "delete": hasDel = true
				}
			}
			if hasSub != c.hasSub { t.Errorf("sub: got %v want %v in %+v", hasSub, c.hasSub, diff) }
			if hasIns != c.hasInsert { t.Errorf("insert: got %v want %v in %+v", hasIns, c.hasInsert, diff) }
			if hasDel != c.hasDelete { t.Errorf("delete: got %v want %v in %+v", hasDel, c.hasDelete, diff) }
		})
	}
}
