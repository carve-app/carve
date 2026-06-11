package audio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestProvidersFor verifies provider selection per language: JA leads with the
// native JapanesePod101 provider then TTS fallback; every other supported
// language gets the TTS provider only.
func TestProvidersFor(t *testing.T) {
	tests := []struct {
		language  string
		wantNames []string
	}{
		{"ja", []string{"jpod101", "tts"}},
		{"zh-cn", []string{"tts"}},
		{"zh-tw", []string{"tts"}},
		{"ko", []string{"tts"}},
		{"en", []string{"tts"}},
		{"es", []string{"tts"}},
		{"de", []string{"tts"}},
		{"fr", []string{"tts"}},
		{"pt", []string{"tts"}},
		{"it", []string{"tts"}},
		{"vi", []string{"tts"}},
		{"unknown", []string{"tts"}}, // chain still has TTS; it no-ops on unknown lang.
	}

	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			chain := providersFor(tt.language)
			var got []string
			for _, p := range chain {
				got = append(got, p.Name())
			}
			if len(got) != len(tt.wantNames) {
				t.Fatalf("providersFor(%q) = %v, want %v", tt.language, got, tt.wantNames)
			}
			for i := range got {
				if got[i] != tt.wantNames[i] {
					t.Fatalf("providersFor(%q)[%d] = %q, want %q", tt.language, i, got[i], tt.wantNames[i])
				}
			}
		})
	}
}

// TestTTSLangCode covers the Carve→Google TTS language-code mapping, including
// the mixed-case zh-CN/zh-TW forms, case-insensitivity, and rejection of
// unsupported languages.
func TestTTSLangCode(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"ja", "ja", true},
		{"zh-cn", "zh-CN", true},
		{"zh-tw", "zh-TW", true},
		{"ZH-CN", "zh-CN", true}, // case-insensitive input.
		{"ko", "ko", true},
		{"en", "en", true},
		{"es", "es", true},
		{"de", "de", true},
		{"fr", "fr", true},
		{"pt", "pt", true},
		{"it", "it", true},
		{"vi", "vi", true},
		{"xx", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := ttsLangCode(tt.in)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("ttsLangCode(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// TestTTSBuildURL asserts the constructed Google Translate TTS URL has the
// required fixed params and a properly-escaped query text.
func TestTTSBuildURL(t *testing.T) {
	got := ttsBuildURL("zh-CN", "你好 世界")

	if !strings.HasPrefix(got, ttsBaseURL+"?") {
		t.Fatalf("URL %q does not start with base %q", got, ttsBaseURL)
	}

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	q := u.Query()

	wantParams := map[string]string{
		"ie":     "UTF-8",
		"client": "tw-ob",
		"tl":     "zh-CN",
		"q":      "你好 世界",
	}
	for k, want := range wantParams {
		if got := q.Get(k); got != want {
			t.Errorf("query param %q = %q, want %q", k, got, want)
		}
	}

	// The space must be percent-encoded in the raw query, never a literal space.
	if strings.Contains(u.RawQuery, " ") {
		t.Errorf("raw query contains an unescaped space: %q", u.RawQuery)
	}
}

// TestTTSEnabledGating verifies the TTS_ENABLED env flag parsing.
func TestTTSEnabledGating(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"", false},     // unset/default → off (so tests never hit the network).
		{"0", false},
		{"false", false},
		{"no", false},
		{"random", false},
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
		{"on", true},
		{"  on  ", true}, // trimmed.
	}

	for _, tt := range tests {
		t.Run("val="+tt.val, func(t *testing.T) {
			t.Setenv("TTS_ENABLED", tt.val)
			if got := ttsEnabled(); got != tt.want {
				t.Fatalf("ttsEnabled() with TTS_ENABLED=%q = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

// TestTTSDisabledReturnsEmpty verifies that with TTS disabled (the test
// default), neither word nor sentence synthesis reaches the network — they
// return "" immediately.
func TestTTSDisabledReturnsEmpty(t *testing.T) {
	t.Setenv("TTS_ENABLED", "")
	p := newTTSProvider()
	if p.Enabled() {
		t.Fatal("expected TTS provider disabled by default")
	}

	ctx := context.Background()
	if url := p.WordAudio(ctx, "es", "hola", ""); url != "" {
		t.Errorf("disabled WordAudio = %q, want empty", url)
	}
	if url := p.synthesize(ctx, "es", "hola mundo"); url != "" {
		t.Errorf("disabled synthesize = %q, want empty", url)
	}
}

// TestTTSSynthesizeWithFakeServer exercises the synthesis HTTP path against an
// httptest server (no real network), covering: success, wrong content-type,
// too-short body, non-200, unsupported language, and over-length text.
func TestTTSSynthesizeWithFakeServer(t *testing.T) {
	const lang = "es"

	newProvider := func(h http.HandlerFunc) (ttsProvider, *httptest.Server) {
		srv := httptest.NewServer(h)
		return ttsProvider{enabled: true, client: srv.Client()}, srv
	}

	// We can't redirect ttsBuildURL's host, so for the happy/edge paths we point
	// the provider's client at the fake server via a custom transport that
	// rewrites the request URL host to the test server.
	makeRewriter := func(target string) http.RoundTripper {
		base := http.DefaultTransport
		tu, _ := url.Parse(target)
		return roundTripFunc(func(r *http.Request) (*http.Response, error) {
			r.URL.Scheme = tu.Scheme
			r.URL.Host = tu.Host
			return base.RoundTrip(r)
		})
	}

	t.Run("success", func(t *testing.T) {
		p, srv := newProvider(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write(make([]byte, ttsMinBytes*2))
		})
		defer srv.Close()
		p.client = &http.Client{Timeout: 2 * time.Second, Transport: makeRewriter(srv.URL)}

		got := p.synthesize(context.Background(), lang, "hola mundo")
		if got == "" {
			t.Fatal("expected non-empty URL on success")
		}
		// Returned URL is the canonical Google TTS URL (the cacheable source).
		if !strings.HasPrefix(got, ttsBaseURL) {
			t.Fatalf("returned URL %q not anchored at TTS base", got)
		}
	})

	t.Run("wrong content type", func(t *testing.T) {
		p, srv := newProvider(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>error</html>"))
		})
		defer srv.Close()
		p.client = &http.Client{Timeout: 2 * time.Second, Transport: makeRewriter(srv.URL)}

		if got := p.synthesize(context.Background(), lang, "hola"); got != "" {
			t.Fatalf("expected empty on non-audio content type, got %q", got)
		}
	})

	t.Run("too short body", func(t *testing.T) {
		p, srv := newProvider(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Write(make([]byte, ttsMinBytes-1))
		})
		defer srv.Close()
		p.client = &http.Client{Timeout: 2 * time.Second, Transport: makeRewriter(srv.URL)}

		if got := p.synthesize(context.Background(), lang, "hola"); got != "" {
			t.Fatalf("expected empty on too-short body, got %q", got)
		}
	})

	t.Run("non-200", func(t *testing.T) {
		p, srv := newProvider(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		defer srv.Close()
		p.client = &http.Client{Timeout: 2 * time.Second, Transport: makeRewriter(srv.URL)}

		if got := p.synthesize(context.Background(), lang, "hola"); got != "" {
			t.Fatalf("expected empty on non-200, got %q", got)
		}
	})

	t.Run("unsupported language", func(t *testing.T) {
		p := ttsProvider{enabled: true, client: &http.Client{Timeout: time.Second}}
		if got := p.synthesize(context.Background(), "xx", "hello"); got != "" {
			t.Fatalf("expected empty on unsupported language, got %q", got)
		}
	})

	t.Run("over-length text", func(t *testing.T) {
		p := ttsProvider{enabled: true, client: &http.Client{Timeout: time.Second}}
		long := strings.Repeat("a", ttsMaxQueryChars+1)
		if got := p.synthesize(context.Background(), lang, long); got != "" {
			t.Fatalf("expected empty on over-length text, got %q", got)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
