package nlp

import (
	"context"
	"strings"
	"testing"
)

// TestBuildExplainPrompt verifies the prompt construction across language and
// sentence permutations. No network is involved.
func TestBuildExplainPrompt(t *testing.T) {
	cases := []struct {
		name           string
		word           string
		sentence       string
		language       string
		nativeLanguage string
		wantContains   []string
		wantOmits      []string
	}{
		{
			name:           "japanese_with_sentence_default_native",
			word:           "食べる",
			sentence:       "りんごを食べる。",
			language:       "ja",
			nativeLanguage: "",
			wantContains:   []string{"Japanese", "食べる", "りんごを食べる。", "in English", "1-2 sentences"},
		},
		{
			name:           "explicit_native_language",
			word:           "走る",
			sentence:       "毎朝走る。",
			language:       "ja",
			nativeLanguage: "es",
			wantContains:   []string{"Japanese", "走る", "in Spanish"},
			wantOmits:      []string{"in English"},
		},
		{
			name:           "no_sentence_omits_sentence_clause",
			word:           "犬",
			sentence:       "",
			language:       "ja",
			nativeLanguage: "en",
			wantContains:   []string{"犬", "Japanese", "in English"},
			wantOmits:      []string{"in this sentence"},
		},
		{
			name:           "whitespace_only_sentence_treated_as_empty",
			word:           "猫",
			sentence:       "   ",
			language:       "ja",
			nativeLanguage: "",
			wantContains:   []string{"猫"},
			wantOmits:      []string{"in this sentence"},
		},
		{
			name:           "unknown_language_code_falls_back_to_code",
			word:           "palabra",
			sentence:       "una palabra nueva",
			language:       "xx",
			nativeLanguage: "en",
			wantContains:   []string{"xx", "palabra", "in English"},
		},
		{
			name:           "empty_language_drops_language_descriptor",
			word:           "word",
			sentence:       "a word here",
			language:       "",
			nativeLanguage: "en",
			wantContains:   []string{`word`, "in English"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildExplainPrompt(tc.word, tc.sentence, tc.language, tc.nativeLanguage)
			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("prompt missing %q\ngot: %s", want, got)
				}
			}
			for _, omit := range tc.wantOmits {
				if strings.Contains(got, omit) {
					t.Errorf("prompt should not contain %q\ngot: %s", omit, got)
				}
			}
		})
	}
}

// TestExplainWordNoKeyGraceful verifies that with no API key configured,
// explainWord returns an empty string and no error (the graceful no-key path),
// without making any network call.
func TestExplainWordNoKeyGraceful(t *testing.T) {
	h := &ExplainHandler{claudeKey: "", claudeURL: "https://api.anthropic.com/v1/messages"}

	got, err := h.explainWord(context.Background(), "食べる", "りんごを食べる。", "ja", "en")
	if err != nil {
		t.Fatalf("expected no error on missing key, got: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty explanation on missing key, got: %q", got)
	}
}

// TestLanguageName covers the known-mapping, fallback-to-code, and empty cases.
func TestLanguageName(t *testing.T) {
	cases := map[string]string{
		"ja":    "Japanese",
		"zh-cn": "Simplified Chinese",
		"ko":    "Korean",
		"xx":    "xx", // unknown → raw code
		"":      "",   // empty → empty
	}
	for code, want := range cases {
		if got := languageName(code); got != want {
			t.Errorf("languageName(%q) = %q, want %q", code, got, want)
		}
	}
}
