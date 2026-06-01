package cards

import (
	"strings"
	"unicode"
)

// charTrigrams returns the set of character trigrams in s after lower-casing
// and collapsing runs of whitespace to a single space. Punctuation is kept —
// for CJK the dominant signal is the characters themselves, and ASCII
// punctuation rarely changes the similarity ranking.
//
// Shorter-than-3 strings get a single key equal to the padded form.
func charTrigrams(s string) map[string]struct{} {
	if s == "" {
		return nil
	}

	// Normalise: lower-case, collapse whitespace.
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) {
			if prevSpace {
				continue
			}
			b.WriteRune(' ')
			prevSpace = true
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	norm := strings.TrimSpace(b.String())
	if norm == "" {
		return nil
	}

	runes := []rune(norm)
	if len(runes) < 3 {
		// Pad short strings so they still produce a single comparable key.
		padded := strings.Repeat(" ", 3-len(runes)) + string(runes)
		return map[string]struct{}{padded: {}}
	}

	out := make(map[string]struct{}, len(runes))
	for i := 0; i+3 <= len(runes); i++ {
		out[string(runes[i:i+3])] = struct{}{}
	}
	return out
}

// jaccardTrigrams returns |A ∩ B| / |A ∪ B|. Empty sets return 0.
func jaccardTrigrams(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// Iterate over the smaller set for the intersection count.
	smaller, larger := a, b
	if len(b) < len(a) {
		smaller, larger = b, a
	}
	inter := 0
	for k := range smaller {
		if _, ok := larger[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
