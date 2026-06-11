package audio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
// providers (exact, native recordings) are tried first; a TTS provider is the
// universal fallback so any supported language still gets word audio.
//
// The generic TTS provider is selected by env (see ttsMode):
//   - "google_cloud": production Cloud TTS (needs GOOGLE_TTS_API_KEY +
//     MEDIA_SERVICE_URL to persist the synthesized bytes as a URL),
//   - "gtts": the key-less Google Translate endpoint (dev/best-effort),
//   - "off": no TTS (default).
func providersFor(language string) []Provider {
	var chain []Provider

	// Japanese keeps its native JapanesePod101 recordings as the primary source.
	if language == "ja" {
		chain = append(chain, jpod101Provider{})
	}

	switch ttsMode() {
	case "google_cloud":
		chain = append(chain, newGoogleCloudTTSProvider())
	case "gtts":
		chain = append(chain, newTTSProvider())
	}
	return chain
}

// ttsMode reports which generic TTS provider to use, driven by env:
//
//	TTS_PROVIDER = google_cloud | gtts | off   (explicit override)
//
// When TTS_PROVIDER is unset/blank the mode is inferred: GOOGLE_TTS_API_KEY set
// → google_cloud; else TTS_ENABLED truthy → gtts; else off.
func ttsMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TTS_PROVIDER"))) {
	case "google_cloud", "gtts", "off":
		return strings.ToLower(strings.TrimSpace(os.Getenv("TTS_PROVIDER")))
	}
	if os.Getenv("GOOGLE_TTS_API_KEY") != "" {
		return "google_cloud"
	}
	if ttsEnabled() {
		return "gtts"
	}
	return "off"
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

// ── Google Cloud Text-to-Speech (production) ──────────────────────────────────
//
// Unlike the key-less gtts endpoint, Cloud TTS is a supported API. It returns
// base64 MP3 bytes (not a URL), so this provider persists the audio through the
// media service and returns that URL. Requires GOOGLE_TTS_API_KEY and
// MEDIA_SERVICE_URL; without both it returns "" (no place to store the bytes).

const googleCloudTTSURL = "https://texttospeech.googleapis.com/v1/text:synthesize"

// cloudTTSBaseURL is overridable in tests to point at an httptest server.
var cloudTTSBaseURL = googleCloudTTSURL

type googleCloudTTSProvider struct {
	apiKey    string
	mediaBase string // e.g. http://media:8002; empty disables persistence.
	mediaTok  string // optional MEDIA_INTERNAL_TOKEN; sent as Bearer when set.
	client    *http.Client
}

func newGoogleCloudTTSProvider() googleCloudTTSProvider {
	return googleCloudTTSProvider{
		apiKey:    os.Getenv("GOOGLE_TTS_API_KEY"),
		mediaBase: strings.TrimRight(os.Getenv("MEDIA_SERVICE_URL"), "/"),
		mediaTok:  os.Getenv("MEDIA_INTERNAL_TOKEN"),
		client:    &http.Client{Timeout: ttsTimeout},
	}
}

func (p googleCloudTTSProvider) Name() string { return "google_cloud_tts" }

// cloudTTSLangCodes maps Carve language codes to BCP-47 codes for Cloud TTS.
var cloudTTSLangCodes = map[string]string{
	"ja": "ja-JP", "zh-cn": "cmn-CN", "zh-tw": "cmn-TW", "ko": "ko-KR",
	"en": "en-US", "es": "es-ES", "de": "de-DE", "fr": "fr-FR",
	"it": "it-IT", "pt": "pt-PT", "vi": "vi-VN",
}

func cloudTTSLangCode(language string) (string, bool) {
	c, ok := cloudTTSLangCodes[strings.ToLower(language)]
	return c, ok
}

type ttsSynthesizeRequest struct {
	Input struct {
		Text string `json:"text"`
	} `json:"input"`
	Voice struct {
		LanguageCode string `json:"languageCode"`
		SSMLGender   string `json:"ssmlGender"`
	} `json:"voice"`
	AudioConfig struct {
		AudioEncoding string `json:"audioEncoding"`
	} `json:"audioConfig"`
}

func (p googleCloudTTSProvider) WordAudio(ctx context.Context, language, lemma, _ string) string {
	if p.apiKey == "" || lemma == "" {
		return ""
	}
	langCode, ok := cloudTTSLangCode(language)
	if !ok {
		return ""
	}
	mp3, err := p.synthesize(ctx, langCode, lemma)
	if err != nil || len(mp3) == 0 {
		if err != nil {
			slog.Warn("google cloud tts synthesize failed", "language", language, "error", err)
		}
		return ""
	}
	if p.mediaBase == "" {
		slog.Warn("google cloud tts: MEDIA_SERVICE_URL unset; cannot persist audio")
		return ""
	}
	storedURL, err := p.uploadToMedia(ctx, mp3)
	if err != nil {
		slog.Warn("google cloud tts: media upload failed", "error", err)
		return ""
	}
	return storedURL
}

// synthesize POSTs to the Cloud TTS REST API and returns decoded MP3 bytes.
func (p googleCloudTTSProvider) synthesize(ctx context.Context, languageCode, text string) ([]byte, error) {
	var body ttsSynthesizeRequest
	body.Input.Text = text
	body.Voice.LanguageCode = languageCode
	body.Voice.SSMLGender = "NEUTRAL"
	body.AudioConfig.AudioEncoding = "MP3"

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint := cloudTTSBaseURL + "?key=" + url.QueryEscape(p.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Don't log the body; it can echo the request text.
		return nil, fmt.Errorf("tts api returned %d", resp.StatusCode)
	}

	var out struct {
		AudioContent string `json:"audioContent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.AudioContent == "" {
		return nil, fmt.Errorf("tts api returned empty audioContent")
	}
	return base64.StdEncoding.DecodeString(out.AudioContent)
}

// uploadToMedia POSTs MP3 bytes to ${MEDIA_SERVICE_URL}/audio and returns the
// absolute media URL. Mirrors cards.uploadToMediaService (replicated here to
// avoid importing the cards package).
func (p googleCloudTTSProvider) uploadToMedia(ctx context.Context, mp3 []byte) (string, error) {
	endpoint := p.mediaBase + "/audio"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(mp3))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "audio/mpeg")
	if p.mediaTok != "" {
		req.Header.Set("Authorization", "Bearer "+p.mediaTok)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("media service %s returned %d", endpoint, resp.StatusCode)
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&result); err != nil {
		return "", err
	}
	if result.URL == "" {
		return "", fmt.Errorf("media service returned empty url")
	}
	return p.mediaBase + result.URL, nil
}
