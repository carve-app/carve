package audio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestCloudTTSLangCode covers the Carve→BCP-47 mapping used by Cloud TTS.
func TestCloudTTSLangCode(t *testing.T) {
	ok := map[string]string{
		"ja": "ja-JP", "zh-cn": "cmn-CN", "zh-tw": "cmn-TW", "ko": "ko-KR",
		"en": "en-US", "es": "es-ES", "de": "de-DE", "fr": "fr-FR",
		"it": "it-IT", "pt": "pt-PT", "vi": "vi-VN",
	}
	for in, want := range ok {
		if got, ok := cloudTTSLangCode(in); !ok || got != want {
			t.Errorf("cloudTTSLangCode(%q) = (%q,%v), want %q", in, got, ok, want)
		}
	}
	if got, ok := cloudTTSLangCode("ZH-CN"); !ok || got != "cmn-CN" {
		t.Errorf("cloudTTSLangCode is not case-insensitive: got (%q,%v)", got, ok)
	}
	if _, ok := cloudTTSLangCode("xx"); ok {
		t.Error("cloudTTSLangCode(xx) should be unsupported")
	}
}

// TestSynthesizeRequestShape drives the low-level Cloud TTS HTTP call against a
// fake server, asserting the request body and that base64 audioContent is
// decoded.
func TestSynthesizeRequestShape(t *testing.T) {
	const raw = "ID3-fake-mp3"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		var req ttsSynthesizeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Voice.LanguageCode != "es-ES" || req.AudioConfig.AudioEncoding != "MP3" || req.Input.Text != "gato" {
			t.Errorf("unexpected request: %+v", req)
		}
		enc := base64.StdEncoding.EncodeToString([]byte(raw))
		w.Write([]byte(`{"audioContent":"` + enc + `"}`))
	}))
	defer srv.Close()

	old := cloudTTSBaseURL
	cloudTTSBaseURL = srv.URL
	defer func() { cloudTTSBaseURL = old }()

	p := newGoogleTTSProvider()
	mp3, err := p.synthesize(context.Background(), "test-token", "es-ES", "gato")
	if err != nil {
		t.Fatalf("synthesize: %v", err)
	}
	if string(mp3) != raw {
		t.Errorf("decoded mp3 = %q, want %q", mp3, raw)
	}
}

// TestSynthesizeErrorPaths covers non-200 and empty-audio responses.
func TestSynthesizeErrorPaths(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"non-200", http.StatusForbidden, `{}`, true},
		{"empty-audio", http.StatusOK, `{"audioContent":""}`, true},
		{"ok", http.StatusOK, `{"audioContent":"` + base64.StdEncoding.EncodeToString([]byte("x")) + `"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			old := cloudTTSBaseURL
			cloudTTSBaseURL = srv.URL
			defer func() { cloudTTSBaseURL = old }()

			_, err := newGoogleTTSProvider().synthesize(context.Background(), "t", "es-ES", "gato")
			if (err != nil) != tc.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

// TestUploadAudioToMedia verifies the MP3 is POSTed to the media service and the
// returned relative URL is qualified with the media base.
func TestUploadAudioToMedia(t *testing.T) {
	const mp3 = "mp3-bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio" {
			t.Errorf("path = %q, want /audio", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "audio/mpeg" {
			t.Errorf("Content-Type = %q, want audio/mpeg", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != mp3 {
			t.Errorf("body = %q, want %q", body, mp3)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"url":"/audio/x.mp3"}`))
	}))
	defer srv.Close()

	got, err := uploadAudioToMedia(context.Background(), srv.Client(), srv.URL, "https://media.public.test", []byte(mp3))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if want := "https://media.public.test/audio/x.mp3"; got != want {
		t.Errorf("upload url = %q, want %q", got, want)
	}
}

// TestSynthesizeGuards verifies the engine no-ops (returns "") without
// credentials / media config, on empty/oversize text, and on unsupported
// languages — never panicking, never producing audio it can't persist.
func TestSynthesizeGuards(t *testing.T) {
	p := newGoogleTTSProvider()
	ctx := context.Background()

	// No GOOGLE_APPLICATION_CREDENTIALS / MEDIA_SERVICE_URL in the test env.
	if url := p.Synthesize(ctx, "es", "gato"); url != "" {
		t.Errorf("unconfigured Synthesize = %q, want empty", url)
	}
	if url := p.Synthesize(ctx, "es", ""); url != "" {
		t.Errorf("empty text = %q, want empty", url)
	}
	if url := p.Synthesize(ctx, "xx", "gato"); url != "" {
		t.Errorf("unsupported lang = %q, want empty", url)
	}
	big := make([]byte, ttsMaxInputBytes+1)
	for i := range big {
		big[i] = 'a'
	}
	if url := p.Synthesize(ctx, "es", string(big)); url != "" {
		t.Errorf("oversize text = %q, want empty", url)
	}
}
