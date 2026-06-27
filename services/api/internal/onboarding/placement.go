package onboarding

import (
	"errors"
	"math"
)

const englishPlacementVersion = "en-receptive-v1"

// PlacementItem is the public, answer-key-free representation sent to the web
// client. The sentence fixes the intended sense of polysemous words without
// turning the task into a decontextualized spelling checklist.
type PlacementItem struct {
	ID      string   `json:"id"`
	Word    string   `json:"word"`
	Prompt  string   `json:"prompt"`
	Options []string `json:"options"`
}

type placementItem struct {
	PlacementItem
	CorrectIndex  int
	Band          int
	FrequencyRank int
}

type PlacementAnswer struct {
	ItemID        string `json:"item_id"`
	SelectedIndex int    `json:"selected_index"`
}

type PlacementBandScore struct {
	Label          string `json:"label"`
	Correct        int    `json:"correct"`
	Total          int    `json:"total"`
	EstimatedKnown int    `json:"estimated_known"`
}

type placementScore struct {
	Correct        int
	Total          int
	EstimatedKnown int
	EstimateLower  int
	EstimateUpper  int
	ResultLabel    string
	CorrectItems   []placementItem
	BandScores     []PlacementBandScore
}

type placementBand struct {
	Label string
	Size  int
}

// The bands intentionally widen above the first 3,000 ranked lemmas. A short
// onboarding diagnostic cannot support the precision of a full 120+ item
// Vocabulary Size Test, so the API returns Wilson-derived bounds as well as the
// point estimate. Item ranks use the same wordfreq Zipf-to-rank approximation
// as Carve's English tokenizer, keeping the diagnostic aligned with subsequent
// content scoring.
var englishPlacementBands = []placementBand{
	{Label: "Most frequent 1,000", Size: 1000},
	{Label: "1,001–2,000", Size: 1000},
	{Label: "2,001–3,000", Size: 1000},
	{Label: "3,001–5,000", Size: 2000},
	{Label: "5,001–8,000", Size: 3000},
	{Label: "8,001–12,000", Size: 4000},
}

var englishPlacementBank = []placementItem{
	{
		PlacementItem: PlacementItem{ID: "en-01-during", Word: "during", Prompt: "No phones are allowed during the exam.", Options: []string{"before something begins", "throughout the time of something", "because of something", "immediately after something"}},
		CorrectIndex:  1, Band: 0, FrequencyRank: 191,
	},
	{
		PlacementItem: PlacementItem{ID: "en-02-happen", Word: "happen", Prompt: "Small mistakes can happen when people are tired.", Options: []string{"become impossible", "take place", "be remembered", "cause damage"}},
		CorrectIndex:  1, Band: 0, FrequencyRank: 646,
	},
	{
		PlacementItem: PlacementItem{ID: "en-03-reason", Word: "reason", Prompt: "What was the reason for the delay?", Options: []string{"the cause or explanation", "the final destination", "the length of time", "the cost of an action"}},
		CorrectIndex:  0, Band: 0, FrequencyRank: 479,
	},
	{
		PlacementItem: PlacementItem{ID: "en-04-voice", Word: "voice", Prompt: "She lowered her voice in the library.", Options: []string{"the words in a book", "the sound made when speaking", "an opinion in writing", "a musical instrument"}},
		CorrectIndex:  1, Band: 0, FrequencyRank: 851,
	},
	{
		PlacementItem: PlacementItem{ID: "en-05-amount", Word: "amount", Prompt: "Add a small amount of salt.", Options: []string{"a type or category", "a container", "a quantity", "a repeated action"}},
		CorrectIndex:  2, Band: 0, FrequencyRank: 741,
	},
	{
		PlacementItem: PlacementItem{ID: "en-06-choose", Word: "choose", Prompt: "You may choose one of the three routes.", Options: []string{"select", "describe", "combine", "remove"}},
		CorrectIndex:  0, Band: 1, FrequencyRank: 1230,
	},
	{
		PlacementItem: PlacementItem{ID: "en-07-effort", Word: "effort", Prompt: "Finishing the project took a great deal of effort.", Options: []string{"luck", "time away", "hard work", "special equipment"}},
		CorrectIndex:  2, Band: 1, FrequencyRank: 1202,
	},
	{
		PlacementItem: PlacementItem{ID: "en-08-avoid", Word: "avoid", Prompt: "Take an earlier train to avoid the crowds.", Options: []string{"watch carefully", "keep away from", "arrive before", "move through"}},
		CorrectIndex:  1, Band: 1, FrequencyRank: 1349,
	},
	{
		PlacementItem: PlacementItem{ID: "en-09-receive", Word: "receive", Prompt: "You will receive a confirmation by email.", Options: []string{"send something back", "ask for something", "be given something", "write something down"}},
		CorrectIndex:  2, Band: 1, FrequencyRank: 1413,
	},
	{
		PlacementItem: PlacementItem{ID: "en-10-reduce", Word: "reduce", Prompt: "The changes should reduce waiting times.", Options: []string{"make shorter or smaller", "measure exactly", "explain clearly", "keep at the same level"}},
		CorrectIndex:  0, Band: 1, FrequencyRank: 1862,
	},
	{
		PlacementItem: PlacementItem{ID: "en-11-concern", Word: "concern", Prompt: "Passenger safety is our main concern.", Options: []string{"a written rule", "a matter that needs attention", "a useful skill", "a reason to celebrate"}},
		CorrectIndex:  1, Band: 2, FrequencyRank: 2188,
	},
	{
		PlacementItem: PlacementItem{ID: "en-12-measure", Word: "measure", Prompt: "We need to measure the width of the doorway.", Options: []string{"guess without checking", "change the shape of", "determine the size of", "draw a picture of"}},
		CorrectIndex:  2, Band: 2, FrequencyRank: 2239,
	},
	{
		PlacementItem: PlacementItem{ID: "en-13-affect", Word: "affect", Prompt: "Cold weather can affect battery life.", Options: []string{"have an influence on", "be a result of", "protect completely", "give a name to"}},
		CorrectIndex:  0, Band: 2, FrequencyRank: 2570,
	},
	{
		PlacementItem: PlacementItem{ID: "en-14-describe", Word: "describe", Prompt: "Can you describe the person you saw?", Options: []string{"ask questions about", "say what someone or something is like", "compare two people", "remember in detail"}},
		CorrectIndex:  1, Band: 2, FrequencyRank: 2754,
	},
	{
		PlacementItem: PlacementItem{ID: "en-15-respond", Word: "respond", Prompt: "Please respond to the invitation by Friday.", Options: []string{"make a decision", "arrive somewhere", "give an answer or reaction", "send a copy"}},
		CorrectIndex:  2, Band: 2, FrequencyRank: 2951,
	},
	{
		PlacementItem: PlacementItem{ID: "en-16-contain", Word: "contain", Prompt: "This bottle may contain traces of nuts.", Options: []string{"include or hold", "be placed beside", "protect from heat", "show the weight of"}},
		CorrectIndex:  0, Band: 3, FrequencyRank: 3162,
	},
	{
		PlacementItem: PlacementItem{ID: "en-17-argue", Word: "argue", Prompt: "The authors argue that the policy should change.", Options: []string{"secretly hope", "give reasons for a position", "discover by accident", "agree without question"}},
		CorrectIndex:  1, Band: 3, FrequencyRank: 3467,
	},
	{
		PlacementItem: PlacementItem{ID: "en-18-establish", Word: "establish", Prompt: "The study aims to establish whether the treatment works.", Options: []string{"hide from view", "make less important", "find out or prove", "repeat from memory"}},
		CorrectIndex:  2, Band: 3, FrequencyRank: 3467,
	},
	{
		PlacementItem: PlacementItem{ID: "en-19-outcome", Word: "outcome", Prompt: "Nobody could predict the outcome of the election.", Options: []string{"the final result", "the first warning", "the main participant", "the official schedule"}},
		CorrectIndex:  0, Band: 3, FrequencyRank: 4467,
	},
	{
		PlacementItem: PlacementItem{ID: "en-20-subsequent", Word: "subsequent", Prompt: "Subsequent experiments confirmed the finding.", Options: []string{"done in secret", "happening later", "less reliable", "using a different method"}},
		CorrectIndex:  1, Band: 3, FrequencyRank: 4074,
	},
	{
		PlacementItem: PlacementItem{ID: "en-21-enable", Word: "enable", Prompt: "The update will enable users to work offline.", Options: []string{"require", "make possible", "make difficult", "prevent"}},
		CorrectIndex:  1, Band: 4, FrequencyRank: 5012,
	},
	{
		PlacementItem: PlacementItem{ID: "en-22-involve", Word: "involve", Prompt: "The role will involve working with several teams.", Options: []string{"include as a necessary part", "be limited to", "happen before", "be easier than"}},
		CorrectIndex:  0, Band: 4, FrequencyRank: 5248,
	},
	{
		PlacementItem: PlacementItem{ID: "en-23-announce", Word: "announce", Prompt: "The company will announce its decision tomorrow.", Options: []string{"delay until later", "consider privately", "make publicly known", "change unexpectedly"}},
		CorrectIndex:  2, Band: 4, FrequencyRank: 5495,
	},
	{
		PlacementItem: PlacementItem{ID: "en-24-depend", Word: "depend", Prompt: "The final cost will depend on how long the work takes.", Options: []string{"be decided or influenced by", "be paid before", "remain lower than", "be added to"}},
		CorrectIndex:  0, Band: 4, FrequencyRank: 6310,
	},
	{
		PlacementItem: PlacementItem{ID: "en-25-acquire", Word: "acquire", Prompt: "Children acquire language through interaction.", Options: []string{"explain", "gain or obtain", "simplify", "forget gradually"}},
		CorrectIndex:  1, Band: 4, FrequencyRank: 7413,
	},
	{
		PlacementItem: PlacementItem{ID: "en-26-inevitable", Word: "inevitable", Prompt: "Some disruption is inevitable during the repairs.", Options: []string{"impossible to avoid", "unlikely to matter", "easy to notice", "carefully planned"}},
		CorrectIndex:  0, Band: 5, FrequencyRank: 8318,
	},
	{
		PlacementItem: PlacementItem{ID: "en-27-adequate", Word: "adequate", Prompt: "The apartment is small but adequate for one person.", Options: []string{"unusually attractive", "temporary", "good enough for the purpose", "available at no cost"}},
		CorrectIndex:  2, Band: 5, FrequencyRank: 8913,
	},
	{
		PlacementItem: PlacementItem{ID: "en-28-abandon", Word: "abandon", Prompt: "The crew had to abandon the damaged ship.", Options: []string{"repair quickly", "leave behind or give up", "move closer to", "take control of"}},
		CorrectIndex:  1, Band: 5, FrequencyRank: 9550,
	},
	{
		PlacementItem: PlacementItem{ID: "en-29-evaluate", Word: "evaluate", Prompt: "The panel will evaluate each proposal.", Options: []string{"publish without changes", "judge the quality or value of", "divide into equal parts", "translate into another language"}},
		CorrectIndex:  1, Band: 5, FrequencyRank: 9772,
	},
	{
		PlacementItem: PlacementItem{ID: "en-30-explicit", Word: "explicit", Prompt: "The instructions make explicit reference to safety.", Options: []string{"open to several interpretations", "added only at the end", "stated clearly and directly", "unlikely to be noticed"}},
		CorrectIndex:  2, Band: 5, FrequencyRank: 10964,
	},
}

func publicEnglishPlacementItems() []PlacementItem {
	items := make([]PlacementItem, 0, len(englishPlacementBank))
	for _, item := range englishPlacementBank {
		items = append(items, item.PlacementItem)
	}
	return items
}

func scoreEnglishPlacement(answers []PlacementAnswer) (placementScore, error) {
	if len(answers) != len(englishPlacementBank) {
		return placementScore{}, errors.New("all placement questions must be answered")
	}

	answerByID := make(map[string]int, len(answers))
	for _, answer := range answers {
		if answer.SelectedIndex < -1 || answer.SelectedIndex > 3 {
			return placementScore{}, errors.New("selected_index must be -1 or an option from 0 to 3")
		}
		if _, exists := answerByID[answer.ItemID]; exists {
			return placementScore{}, errors.New("placement answers contain duplicate item ids")
		}
		answerByID[answer.ItemID] = answer.SelectedIndex
	}

	correctByBand := make([]int, len(englishPlacementBands))
	totalByBand := make([]int, len(englishPlacementBands))
	score := placementScore{Total: len(englishPlacementBank)}
	for _, item := range englishPlacementBank {
		selected, exists := answerByID[item.ID]
		if !exists {
			return placementScore{}, errors.New("placement answers contain an unknown or missing item id")
		}
		totalByBand[item.Band]++
		if selected == item.CorrectIndex {
			score.Correct++
			correctByBand[item.Band]++
			score.CorrectItems = append(score.CorrectItems, item)
		}
	}

	for i, band := range englishPlacementBands {
		correct := correctByBand[i]
		total := totalByBand[i]
		proportion := float64(correct) / float64(total)
		estimated := int(math.Round(proportion*float64(band.Size)/100.0) * 100)
		lower, upper := wilsonInterval(correct, total, 1.645) // honest 90% range
		score.EstimatedKnown += estimated
		score.EstimateLower += int(math.Round(lower*float64(band.Size)/100.0) * 100)
		score.EstimateUpper += int(math.Round(upper*float64(band.Size)/100.0) * 100)
		score.BandScores = append(score.BandScores, PlacementBandScore{
			Label: band.Label, Correct: correct, Total: total, EstimatedKnown: estimated,
		})
	}

	if score.EstimateLower < 0 {
		score.EstimateLower = 0
	}
	if score.EstimateUpper > 12000 {
		score.EstimateUpper = 12000
	}
	score.ResultLabel = placementResultLabel(score.EstimatedKnown)
	return score, nil
}

func wilsonInterval(successes, trials int, z float64) (float64, float64) {
	if trials == 0 {
		return 0, 1
	}
	n := float64(trials)
	p := float64(successes) / n
	z2 := z * z
	center := (p + z2/(2*n)) / (1 + z2/n)
	margin := z * math.Sqrt((p*(1-p)+z2/(4*n))/n) / (1 + z2/n)
	return math.Max(0, center-margin), math.Min(1, center+margin)
}

func placementResultLabel(estimate int) string {
	switch {
	case estimate < 1500:
		return "Foundation"
	case estimate < 3000:
		return "Developing"
	case estimate < 5000:
		return "Independent"
	case estimate < 8000:
		return "Strong"
	default:
		return "Advanced"
	}
}
