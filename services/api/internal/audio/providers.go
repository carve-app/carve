package audio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Provider resolves word-pronunciation audio for a single lemma. Each provider
// is responsible for verifying it actually produced audio; WordAudio MUST
// return "" (not a guessed URL) when it cannot supply validated audio.
type Provider interface {
	// Name is the stable cache key recorded in audio_cache.provider.
	Name() string
	// WordAudio returns a validated audio URL for the lemma, or "".
	WordAudio(ctx context.Context, language, lemma, reading string) string
}

// providersFor returns the ordered provider chain for a language. Dictionary
// providers (exact, native recordings) are tried first; the TTS provider is
// the universal fallback so any supported language still gets word audio.
func providersFor(language string) []Provider {
	var chain []Provider

	// Japanese keeps its native JapanesePod101 recordings as the primary source
	// — unchanged from before this refactor.
	if language == "ja" {
		chain = append(chain, jpod101Provider{})
	}

	// TTS is the cross-language fallback. It only acts when enabled and the
	// language is mappable (see WordAudio), so adding it here is harmless when
	// TTS is off.
	chain = append(chain, newTTSProvider())

	return chain
}

// ── JapanesePod101 (Japanese only) ────────────────────────────────────────────

const (
	// JapanesePod101 free audio URL pattern; does not require an API key.
	jpod101URL = "https://assets.languagepod101.com/dictionary/japanese/audiomp3/?kana=%s&kanji=%s"
	// Probe timeout — we only send HEAD to verify the URL returns audio/mpeg.
	probeTimeout = 5 * time.Second
)

var probeClient = &http.Client{Timeout: probeTimeout}

type jpod101Provider struct{}

func (jpod101Provider) Name() string { return "jpod101" }

// WordAudio reproduces the original JapanesePod101 behaviour exactly: it
// requires a reading, constructs the kana/kanji URL, HEAD-probes it, and
// rejects the ~52-byte silent placeholder MP3 that the endpoint serves for
// missing entries.
func (jpod101Provider) WordAudio(ctx context.Context, language, lemma, reading string) string {
	if language != "ja" || reading == "" || lemma == "" {
		return ""
	}

	audioURL := fmt.Sprintf(jpod101URL, url.QueryEscape(reading), url.QueryEscape(lemma))

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, audioURL, nil)
	if err != nil {
		return ""
	}
	resp, err := probeClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "audio/mpeg" && ct != "audio/mp3" {
		// JapanesePod101 returns a silent MP3 with Content-Length:52 for missing
		// entries; anything shorter than ~1 kB is the "not found" placeholder.
		if resp.ContentLength > 0 && resp.ContentLength < 1024 {
			return ""
		}
	}
	return audioURL
}

// ── Text-to-speech (Google Translate, key-less) ───────────────────────────────

const (
	// ttsBaseURL is Google Translate's undocumented but widely-used free TTS
	// endpoint. The tw-ob client returns an audio/mpeg stream. It is NOT a
	// supported/SLA'd API — see package docs and the task notes; it is a
	// pragmatic, no-API-key option used by many OSS tools.
	ttsBaseURL = "https://translate.google.com/translate_tts"
	// ttsTimeout bounds a single synthesis request.
	ttsTimeout = 8 * time.Second
	// ttsMaxQueryChars is Google's per-request text limit; longer text is
	// rejected by the endpoint, so we refuse it up front.
	ttsMaxQueryChars = 200
	// ttsMinBytes guards against truncated/empty/HTML error responses being
	// cached as if they were real audio.
	ttsMinBytes = 256
)

// ttsLangCodes maps Carve's internal language codes to Google TTS language
// codes. Carve uses lowercase region tags (zh-cn/zh-tw); Google expects the
// mixed-case BCP-47-ish forms.
var ttsLangCodes = map[string]string{
	"ja":    "ja",
	"zh-cn": "zh-CN",
	"zh-tw": "zh-TW",
	"ko":    "ko",
	"en":    "en",
	"es":    "es",
	"de":    "de",
	"fr":    "fr",
	"pt":    "pt",
	"it":    "it",
	"vi":    "vi",
}

// ttsLangCode returns the Google TTS language code for a Carve language code.
// The bool is false for unsupported languages.
func ttsLangCode(language string) (string, bool) {
	code, ok := ttsLangCodes[strings.ToLower(language)]
	return code, ok
}

// ttsBuildURL constructs the Google Translate TTS request URL for the given
// text. Exposed (lowercase, package-internal) so tests can assert URL shape
// without hitting the network.
func ttsBuildURL(googleLang, text string) string {
	q := url.Values{}
	q.Set("ie", "UTF-8")
	q.Set("client", "tw-ob")
	q.Set("tl", googleLang)
	q.Set("q", text)
	return ttsBaseURL + "?" + q.Encode()
}

// ttsProvider synthesizes audio via Google Translate TTS. It is gated behind
// the TTS_ENABLED env flag (default off, so tests never reach the network) and
// requires a mappable language.
type ttsProvider struct {
	enabled bool
	client  *http.Client
}

func newTTSProvider() ttsProvider {
	return ttsProvider{
		enabled: ttsEnabled(),
		client:  &http.Client{Timeout: ttsTimeout},
	}
}

// ttsEnabled reports the TTS_ENABLED env flag. Default (unset/empty) is off.
func ttsEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TTS_ENABLED"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (p ttsProvider) Name() string { return "tts" }

func (p ttsProvider) Enabled() bool { return p.enabled }

// WordAudio synthesizes audio for a single word/lemma. Returns "" when TTS is
// disabled or the language is unsupported.
func (p ttsProvider) WordAudio(ctx context.Context, language, lemma, _ string) string {
	if !p.enabled || lemma == "" {
		return ""
	}
	return p.synthesize(ctx, language, lemma)
}

// synthesize fetches audio for arbitrary text, validates it is real audio, and
// returns the request URL on success (or "" on any failure). The returned URL
// is stable for the same text, so it doubles as the cached, replayable source.
func (p ttsProvider) synthesize(ctx context.Context, language, text string) string {
	if !p.enabled || text == "" {
		return ""
	}
	googleLang, ok := ttsLangCode(language)
	if !ok {
		return ""
	}
	// The endpoint rejects overly long text; refuse rather than send garbage.
	if len([]rune(text)) > ttsMaxQueryChars {
		return ""
	}

	audioURL := ttsBuildURL(googleLang, text)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return ""
	}
	// The endpoint serves audio only to browser-like clients.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CarveBot/1.0)")

	resp, err := p.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return ""
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "audio/") {
		return ""
	}

	// Read a bounded prefix to confirm we actually received audio bytes (guards
	// against empty / HTML error bodies served with an audio content type).
	buf := make([]byte, ttsMinBytes)
	n, _ := io.ReadFull(resp.Body, buf)
	if n < ttsMinBytes {
		return ""
	}

	return audioURL
}
