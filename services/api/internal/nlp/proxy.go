package nlp

import (
	"io"
	"log/slog"
	"net/http"
	"os"
)

// Proxy forwards NLP requests to the Python NLP service.
type Proxy struct {
	serviceURL     string
	internalSecret string
}

func NewProxy() *Proxy {
	url := os.Getenv("NLP_SERVICE_URL")
	if url == "" {
		url = "http://localhost:8001"
	}
	return &Proxy{
		serviceURL:     url,
		internalSecret: os.Getenv("NLP_INTERNAL_SECRET"),
	}
}

// forward proxies the incoming request body to the given upstream path,
// copies all response headers and the body back to the client.
func (p *Proxy) forward(w http.ResponseWriter, r *http.Request, upstreamPath string) {
	upstreamURL := p.serviceURL + upstreamPath

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, r.Body)
	if err != nil {
		slog.Error("nlp proxy: build request", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Forward content-type from the original request.
	if ct := r.Header.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}

	// Attach internal shared secret if configured.
	if p.internalSecret != "" {
		req.Header.Set("X-Internal-Secret", p.internalSecret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("nlp proxy: upstream request failed", "url", upstreamURL, "error", err)
		http.Error(w, `{"error":"nlp service unavailable"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers (e.g. Content-Type).
	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Error("nlp proxy: copy response body", "error", err)
	}
}

// POST /v1/nlp/tokenize
func (p *Proxy) Tokenize(w http.ResponseWriter, r *http.Request) {
	p.forward(w, r, "/tokenize")
}

// POST /v1/nlp/lookup
func (p *Proxy) Lookup(w http.ResponseWriter, r *http.Request) {
	p.forward(w, r, "/lookup")
}
