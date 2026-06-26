package nlp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func TestProxyForwardsBodyAndInternalSecret(t *testing.T) {
	client := doerFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "http://nlp.test/tokenize" {
			t.Fatalf("unexpected upstream URL: %s", req.URL)
		}
		if got := req.Header.Get("X-Internal-Secret"); got != "secret" {
			t.Fatalf("missing internal secret: %q", got)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != `{"text":"cold start"}` {
			t.Fatalf("unexpected body: %s", body)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"tokens":[]}`)),
		}, nil
	})
	p := NewProxyWithClient("http://nlp.test", "secret", client)
	req := httptest.NewRequest(http.MethodPost, "/v1/nlp/tokenize", strings.NewReader(`{"text":"cold start"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	p.Tokenize(w, req)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"tokens":[]}` {
		t.Fatalf("unexpected proxy response: %d %s", w.Code, w.Body.String())
	}
}

func TestProxyCancelsUpstreamWhenClientDisconnects(t *testing.T) {
	client := doerFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	p := NewProxyWithClient("http://nlp.test", "", client)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/nlp/tokenize", strings.NewReader(`{}`)).WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		p.Tokenize(w, req)
		close(done)
	}()
	cancel()
	<-done
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 after cancellation, got %d", w.Code)
	}
}

func TestProxyReportsProviderFailure(t *testing.T) {
	p := NewProxyWithClient("http://nlp.test", "", doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("provider unavailable")
	}))
	w := httptest.NewRecorder()
	p.Lookup(w, httptest.NewRequest(http.MethodPost, "/v1/nlp/lookup", strings.NewReader(`{}`)))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", w.Code)
	}
}

func TestProxyRejectsMalformedOrOversizedProviderResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed json", body: `{"tokens":`},
		{name: "oversized", body: strings.Repeat("x", nlpMaxResponseBytes+1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewProxyWithClient("http://nlp.test", "", doerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tc.body)),
				}, nil
			}))
			w := httptest.NewRecorder()
			p.Lookup(w, httptest.NewRequest(http.MethodPost, "/v1/nlp/lookup", strings.NewReader(`{}`)))
			if w.Code != http.StatusBadGateway {
				t.Fatalf("expected 502, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestMergeStringsDeduplicatesAndExcludesKnown(t *testing.T) {
	got := mergeStrings(
		[]any{"existing", "known", "existing", 42},
		[]string{"added", "known", "added"},
		map[string]bool{"known": true},
	)
	want := []string{"existing", "added"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeStrings() = %#v, want %#v", got, want)
	}
}
