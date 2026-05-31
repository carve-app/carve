package output

import (
	"math"
	"testing"
)

func TestComputeDiff_Identical(t *testing.T) {
	diff, wer := computeDiff("hello world", "hello world")
	if wer != 0 {
		t.Errorf("expected WER 0, got %v", wer)
	}
	if len(diff) != 2 {
		t.Errorf("expected 2 diff entries, got %d", len(diff))
	}
	for _, e := range diff {
		if e.Op != "keep" {
			t.Errorf("expected keep, got %s", e.Op)
		}
	}
}

func TestComputeDiff_Empty(t *testing.T) {
	diff, wer := computeDiff("", "")
	if wer != 0 || len(diff) != 0 {
		t.Errorf("expected empty diff with WER 0, got diff=%v wer=%v", diff, wer)
	}
}

func TestComputeDiff_Substitution(t *testing.T) {
	diff, wer := computeDiff("the quick brown fox", "the quick brown dog")
	expected := 1.0 / 4.0
	if math.Abs(wer-expected) > 0.0001 {
		t.Errorf("expected WER %.4f, got %.4f", expected, wer)
	}
	var hasSub bool
	for _, e := range diff {
		if e.Op == "sub" && e.Ref == "fox" && e.Hyp == "dog" {
			hasSub = true
		}
	}
	if !hasSub {
		t.Errorf("expected sub entry for fox→dog, got %+v", diff)
	}
}

func TestComputeDiff_Insertion(t *testing.T) {
	_, wer := computeDiff("hello world", "hello big world")
	expected := 1.0 / 2.0
	if math.Abs(wer-expected) > 0.0001 {
		t.Errorf("expected WER %.4f, got %.4f", expected, wer)
	}
}

func TestComputeDiff_Deletion(t *testing.T) {
	_, wer := computeDiff("hello big world", "hello world")
	expected := 1.0 / 3.0
	if math.Abs(wer-expected) > 0.0001 {
		t.Errorf("expected WER %.4f, got %.4f", expected, wer)
	}
}

func TestComputeDiff_PunctuationNormalization(t *testing.T) {
	_, wer := computeDiff("Hello, world!", "hello world")
	if wer != 0 {
		t.Errorf("punctuation/casing should normalize: got WER %v", wer)
	}
}

func TestNormalize_DropsEmptyTokens(t *testing.T) {
	got := normalize("  the   cat   sat  ")
	if len(got) != 3 || got[0] != "the" || got[1] != "cat" || got[2] != "sat" {
		t.Errorf("unexpected normalize result: %v", got)
	}
}
