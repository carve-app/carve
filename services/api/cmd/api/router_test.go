package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

func TestRouterPublicAndProtectedBoundaries(t *testing.T) {
	r := newRouter(nil)

	for _, path := range []string{"/health", "/health/live"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", path, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/cards", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("protected route: expected 401, got %d", w.Code)
	}
}

func TestOpenAPICoversEveryRuntimeRoute(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "contract-test")
	r := newRouter(nil)
	runtime := map[string]bool{}
	if err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		runtime[strings.ToLower(method)+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse OpenAPI: %v", err)
	}
	documented := map[string]bool{}
	for path, methods := range spec.Paths {
		for method := range methods {
			switch strings.ToLower(method) {
			case "get", "post", "put", "patch", "delete":
				documented[strings.ToLower(method)+" "+path] = true
			}
		}
	}

	var missing, stale []string
	for route := range runtime {
		if !documented[route] {
			missing = append(missing, route)
		}
	}
	for route := range documented {
		if !runtime[route] {
			stale = append(stale, route)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("OpenAPI route drift\nmissing: %v\nstale: %v", missing, stale)
	}
}

func TestRouterProtectsMetricsWhenConfigured(t *testing.T) {
	t.Setenv("METRICS_TOKEN", "test-metrics-token")
	r := newRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without metrics token, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer test-metrics-token")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with metrics token, got %d", w.Code)
	}
}
