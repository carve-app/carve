package audio

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Audio is synthesized by a single best-on-market engine: Google Cloud
// Text-to-Speech (Chirp3-HD / Neural2 voices), authenticated with a service
// account. There is deliberately NO scraped/keyless fallback — when the engine
// is not configured, audio is simply absent rather than lower quality.
//
// Config (Application Default Credentials):
//   GOOGLE_APPLICATION_CREDENTIALS = path to service-account JSON
//   GOOGLE_CLOUD_PROJECT           = project id (optional; taken from the SA)
//   MEDIA_SERVICE_URL              = where synthesized MP3s are persisted
//   MEDIA_INTERNAL_TOKEN           = bearer for the media service (optional)

const (
	cloudTTSURL = "https://texttospeech.googleapis.com/v1/text:synthesize"
	ttsTimeout  = 15 * time.Second
	// Cloud TTS hard-caps a single request at 5000 bytes of input.
	ttsMaxInputBytes = 5000
)

// cloudTTSBaseURL is overridable in tests to point at an httptest server.
var cloudTTSBaseURL = cloudTTSURL

// cloudTTSLangCodes maps Carve language codes to BCP-47 codes for Cloud TTS.
var cloudTTSLangCodes = map[string]string{
	"ja": "ja-JP", "zh-cn": "cmn-CN", "zh-tw": "cmn-TW", "zh": "cmn-CN",
	"ko": "ko-KR", "en": "en-US", "es": "es-ES", "de": "de-DE",
	"fr": "fr-FR", "it": "it-IT", "pt": "pt-PT", "vi": "vi-VN",
}

func cloudTTSLangCode(language string) (string, bool) {
	c, ok := cloudTTSLangCodes[strings.ToLower(language)]
	return c, ok
}

// ── Service-account token source (lazy, process-wide, auto-refreshing) ─────────

var (
	tokenOnce   sync.Once
	tokenSource oauth2.TokenSource
	saProjectID string
)

// loadTokenSource builds an auto-refreshing OAuth2 token source from the
// service-account JSON named by GOOGLE_APPLICATION_CREDENTIALS. Cached for the
// process. Returns nil when credentials are unavailable.
func loadTokenSource(ctx context.Context) (oauth2.TokenSource, string) {
	tokenOnce.Do(func() {
		path := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
		if path == "" {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		// The deprecation notice on this API warns about accepting credential
		// JSON from untrusted sources; here the path is operator-controlled via
		// GOOGLE_APPLICATION_CREDENTIALS, so the risk does not apply. Using this
		// instead of pulling in the full cloud.google.com/go client stack.
		creds, err := google.CredentialsFromJSONWithParams(ctx, data, google.CredentialsParams{
			Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"},
		})
		if err != nil {
			return
		}
		tokenSource = creds.TokenSource
		saProjectID = creds.ProjectID
		if p := os.Getenv("GOOGLE_CLOUD_PROJECT"); p != "" {
			saProjectID = p
		}
	})
	return tokenSource, saProjectID
}

// ── Google Cloud TTS provider ──────────────────────────────────────────────

type googleTTSProvider struct {
	client *http.Client
}

func newGoogleTTSProvider() googleTTSProvider {
	return googleTTSProvider{client: &http.Client{Timeout: ttsTimeout}}
}

func (googleTTSProvider) Name() string { return "google_cloud_tts" }

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

// Synthesize generates MP3 audio for arbitrary text in the given language,
// persists it via the media service, and returns the stored URL. Returns ""
// when the engine is unconfigured, the language is unsupported, the text is too
// long, or any step fails.
func (p googleTTSProvider) Synthesize(ctx context.Context, language, text string) string {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > ttsMaxInputBytes {
		return ""
	}
	langCode, ok := cloudTTSLangCode(language)
	if !ok {
		return ""
	}
	ts, _ := loadTokenSource(ctx)
	if ts == nil {
		return ""
	}
	mediaBase := strings.TrimRight(os.Getenv("MEDIA_SERVICE_URL"), "/")
	if mediaBase == "" {
		return ""
	}
	tok, err := ts.Token()
	if err != nil {
		return ""
	}

	mp3, err := p.synthesize(ctx, tok.AccessToken, langCode, text)
	if err != nil || len(mp3) == 0 {
		return ""
	}
	publicBase := strings.TrimRight(os.Getenv("MEDIA_PUBLIC_BASE"), "/")
	if publicBase == "" {
		publicBase = mediaBase
	}
	url, err := uploadAudioToMedia(ctx, p.client, mediaBase, publicBase, mp3)
	if err != nil {
		return ""
	}
	return url
}

// WordAudio synthesizes pronunciation for a single lemma.
func (p googleTTSProvider) WordAudio(ctx context.Context, language, lemma, _ string) string {
	return p.Synthesize(ctx, language, lemma)
}

func (p googleTTSProvider) synthesize(ctx context.Context, token, langCode, text string) ([]byte, error) {
	var body ttsSynthesizeRequest
	body.Input.Text = text
	body.Voice.LanguageCode = langCode
	body.Voice.SSMLGender = "NEUTRAL"
	body.AudioConfig.AudioEncoding = "MP3"

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cloudTTSBaseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloud tts returned %d", resp.StatusCode)
	}

	var out struct {
		AudioContent string `json:"audioContent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.AudioContent == "" {
		return nil, fmt.Errorf("cloud tts returned empty audioContent")
	}
	return base64.StdEncoding.DecodeString(out.AudioContent)
}

// uploadAudioToMedia POSTs MP3 bytes to ${MEDIA_SERVICE_URL}/audio and returns
// the absolute media URL. Mirrors cards.uploadToMediaService (replicated here
// to avoid importing the cards package).
func uploadAudioToMedia(ctx context.Context, client *http.Client, mediaBase, publicBase string, mp3 []byte) (string, error) {
	endpoint := mediaBase + "/audio"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(mp3))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "audio/mpeg")
	if tok := os.Getenv("MEDIA_INTERNAL_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
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
	if strings.HasPrefix(result.URL, "http://") || strings.HasPrefix(result.URL, "https://") {
		return result.URL, nil
	}
	return strings.TrimRight(publicBase, "/") + "/" + strings.TrimLeft(result.URL, "/"), nil
}
