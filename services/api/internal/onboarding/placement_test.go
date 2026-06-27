package onboarding

import "testing"

func placementAnswers(selectAnswer func(placementItem) int) []PlacementAnswer {
	answers := make([]PlacementAnswer, 0, len(englishPlacementBank))
	for _, item := range englishPlacementBank {
		answers = append(answers, PlacementAnswer{
			ItemID: item.ID, SelectedIndex: selectAnswer(item),
		})
	}
	return answers
}

func TestScoreEnglishPlacementAllCorrect(t *testing.T) {
	score, err := scoreEnglishPlacement(placementAnswers(func(item placementItem) int {
		return item.CorrectIndex
	}))
	if err != nil {
		t.Fatal(err)
	}
	if score.Correct != len(englishPlacementBank) || len(score.CorrectItems) != len(englishPlacementBank) {
		t.Fatalf("correct=%d verified=%d, want %d", score.Correct, len(score.CorrectItems), len(englishPlacementBank))
	}
	if score.EstimatedKnown != 12000 || score.ResultLabel != "Advanced" {
		t.Fatalf("estimate=%d label=%q, want 12000 Advanced", score.EstimatedKnown, score.ResultLabel)
	}
	if score.EstimateLower <= 0 || score.EstimateLower >= score.EstimatedKnown {
		t.Fatalf("lower bound %d should be honest and below point estimate", score.EstimateLower)
	}
	if score.EstimateUpper != 12000 {
		t.Fatalf("upper bound=%d, want 12000", score.EstimateUpper)
	}
}

func TestScoreEnglishPlacementUnknownAnswers(t *testing.T) {
	score, err := scoreEnglishPlacement(placementAnswers(func(placementItem) int { return -1 }))
	if err != nil {
		t.Fatal(err)
	}
	if score.Correct != 0 || score.EstimatedKnown != 0 || score.ResultLabel != "Foundation" {
		t.Fatalf("unexpected empty score: %+v", score)
	}
	if score.EstimateLower != 0 || score.EstimateUpper <= 0 {
		t.Fatalf("expected a non-zero uncertainty range, got %d–%d", score.EstimateLower, score.EstimateUpper)
	}
}

func TestScoreEnglishPlacementUsesStratifiedBandWeights(t *testing.T) {
	// Knowing the first three 1,000-item bands but none above them should
	// produce a 3,000-word estimate rather than treating every item as an
	// equal share of the 12,000-word range.
	score, err := scoreEnglishPlacement(placementAnswers(func(item placementItem) int {
		if item.Band < 3 {
			return item.CorrectIndex
		}
		return -1
	}))
	if err != nil {
		t.Fatal(err)
	}
	if score.EstimatedKnown != 3000 || score.ResultLabel != "Independent" {
		t.Fatalf("estimate=%d label=%q, want 3000 Independent", score.EstimatedKnown, score.ResultLabel)
	}
}

func TestScoreEnglishPlacementRejectsMalformedAnswers(t *testing.T) {
	valid := placementAnswers(func(placementItem) int { return -1 })

	if _, err := scoreEnglishPlacement(valid[:len(valid)-1]); err == nil {
		t.Fatal("expected missing-answer error")
	}

	duplicate := append([]PlacementAnswer(nil), valid...)
	duplicate[len(duplicate)-1].ItemID = duplicate[0].ItemID
	if _, err := scoreEnglishPlacement(duplicate); err == nil {
		t.Fatal("expected duplicate-answer error")
	}

	invalidChoice := append([]PlacementAnswer(nil), valid...)
	invalidChoice[0].SelectedIndex = 4
	if _, err := scoreEnglishPlacement(invalidChoice); err == nil {
		t.Fatal("expected selected-index error")
	}
}

func TestPublicEnglishPlacementItemsDoNotExposeAnswers(t *testing.T) {
	items := publicEnglishPlacementItems()
	if len(items) != len(englishPlacementBank) {
		t.Fatalf("got %d public items, want %d", len(items), len(englishPlacementBank))
	}
	for _, item := range items {
		if item.ID == "" || item.Word == "" || item.Prompt == "" || len(item.Options) != 4 {
			t.Fatalf("incomplete public item: %+v", item)
		}
	}
}
