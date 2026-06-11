package nlp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWikiHostForLanguage(t *testing.T) {
	cases := []struct {
		language string
		want     string
	}{
		{"ja", "ja.wikipedia.org"},
		{"en", "en.wikipedia.org"},
		{"es", "es.wikipedia.org"},
		{"de", "de.wikipedia.org"},
		{"fr", "fr.wikipedia.org"},
		{"it", "it.wikipedia.org"},
		{"pt", "pt.wikipedia.org"},
		{"zh", "zh.wikipedia.org"},
		{"ko", "ko.wikipedia.org"},
		{"", "en.wikipedia.org"},        // empty falls back to English
		{"klingon", "en.wikipedia.org"}, // unknown falls back to English
	}
	for _, c := range cases {
		if got := wikiHostForLanguage(c.language); got != c.want {
			t.Errorf("wikiHostForLanguage(%q) = %q, want %q", c.language, got, c.want)
		}
	}
}

// withWikiBase points the image lookup at a test server for the duration of fn.
func withWikiBase(base string, fn func()) {
	prev := wordImageBaseOverride
	wordImageBaseOverride = base
	defer func() { wordImageBaseOverride = prev }()
	fn()
}

func decodeImageURL(t *testing.T, rr *httptest.ResponseRecorder) any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v (body=%q)", err, rr.Body.String())
	}
	v, ok := body["image_url"]
	if !ok {
		t.Fatalf("response missing image_url key: %q", rr.Body.String())
	}
	return v
}

func TestWordImage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"thumbnail":{"source":"https://example.org/cat.jpg"}}`))
	}))
	defer srv.Close()

	withWikiBase(srv.URL, func() {
		p := NewProxy()
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/nlp/word-image?word=猫&language=ja", nil)
		p.WordImage(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		if got := decodeImageURL(t, rr); got != "https://example.org/cat.jpg" {
			t.Errorf("image_url = %v, want the thumbnail source", got)
		}
	})
}

func TestWordImage_NullPaths(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		handler http.HandlerFunc
	}{
		{
			name:  "missing word never hits upstream",
			query: "?language=ja",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				t.Error("upstream should not be called when word is empty")
			},
		},
		{
			name:  "upstream 404 (no wikipedia page)",
			query: "?word=zzzznotaword&language=en",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
		},
		{
			name:  "summary has no thumbnail",
			query: "?word=abstractconcept&language=en",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"title":"Abstract concept"}`))
			},
		},
		{
			name:  "upstream returns garbage",
			query: "?word=broken&language=en",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`not json at all`))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			withWikiBase(srv.URL, func() {
				p := NewProxy()
				rr := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodGet, "/v1/nlp/word-image"+tc.query, nil)
				p.WordImage(rr, req)

				if rr.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200 (failures must be graceful)", rr.Code)
				}
				if got := decodeImageURL(t, rr); got != nil {
					t.Errorf("image_url = %v, want null on failure", got)
				}
			})
		})
	}
}
