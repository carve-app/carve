package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/carve-app/carve/services/media/internal/storage"
)

func mustIP(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("bad test IP %q", s)
	}
	return ip
}

// memStore is an in-memory storage.Storage for tests.
type memStore struct{ objects map[string][]byte }

func newMemStore() *memStore { return &memStore{objects: map[string][]byte{}} }

func (m *memStore) Put(_ context.Context, prefix, key, _ string, body io.Reader, _ int64) (string, error) {
	b, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	m.objects[prefix+"/"+key] = b
	return "/" + prefix + "/" + key, nil
}

func (m *memStore) Open(_ context.Context, prefix, key string) (io.ReadCloser, string, error) {
	b, ok := m.objects[prefix+"/"+key]
	if !ok {
		return nil, "", storage.ErrUnsupported
	}
	return io.NopCloser(bytes.NewReader(b)), "application/octet-stream", nil
}

func TestImageExt(t *testing.T) {
	cases := map[string]struct {
		ext string
		ok  bool
	}{
		"image/png":         {".png", true},
		"image/jpeg":        {".jpg", true},
		"image/webp":        {".webp", true},
		"image/jpeg; q=0.8": {".jpg", true},
		"application/octet": {"", false},
		"text/html":         {"", false},
	}
	for ct, want := range cases {
		ext, ok := imageExt(ct)
		if ok != want.ok || ext != want.ext {
			t.Errorf("imageExt(%q) = (%q,%v), want (%q,%v)", ct, ext, ok, want.ext, want.ok)
		}
	}
}

func TestAuthorizeWrite(t *testing.T) {
	// No token configured → open writes.
	if !authorizeWrite(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/screenshots", nil), "") {
		t.Error("empty token should allow writes (dev mode)")
	}

	// Token configured → require matching bearer.
	tok := "secret-token"
	mk := func(auth string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/screenshots", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		return r
	}
	if authorizeWrite(httptest.NewRecorder(), mk(""), tok) {
		t.Error("missing Authorization must be rejected")
	}
	if authorizeWrite(httptest.NewRecorder(), mk("Bearer wrong"), tok) {
		t.Error("wrong token must be rejected")
	}
	if !authorizeWrite(httptest.NewRecorder(), mk("Bearer "+tok), tok) {
		t.Error("correct token must be accepted")
	}
}

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", "10.0.0.5", "192.168.1.1", "172.16.0.1",
		"169.254.169.254", // cloud metadata
		"169.254.170.2",   // fargate task-role endpoint
		"100.64.0.1",      // CGNAT
		"0.0.0.0",
		"::ffff:169.254.169.254", // IPv4-mapped metadata
	}
	for _, s := range blocked {
		if !isBlockedIP(mustIP(t, s)) {
			t.Errorf("isBlockedIP(%s) = false, want true", s)
		}
	}
	public := []string{"1.1.1.1", "8.8.8.8", "93.184.216.34"}
	for _, s := range public {
		if isBlockedIP(mustIP(t, s)) {
			t.Errorf("isBlockedIP(%s) = true, want false", s)
		}
	}
}

// TestUploadRejectsOversize verifies an oversized body is rejected (413) rather
// than silently truncated, and that an unsupported content type is rejected.
func TestUploadHandlerValidation(t *testing.T) {
	store := newMemStore()
	h := uploadHandler("screenshots", 1<<10, imageExt, store) // 1KiB cap (test-only)

	// Unsupported content type → 415.
	r := httptest.NewRequest(http.MethodPost, "/screenshots", strings.NewReader("x"))
	r.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("bad content type: got %d, want 415", w.Code)
	}

	// Oversized body → 413, nothing stored.
	big := bytes.Repeat([]byte("a"), (1<<10)+1)
	r = httptest.NewRequest(http.MethodPost, "/screenshots", bytes.NewReader(big))
	r.Header.Set("Content-Type", "image/png")
	w = httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: got %d, want 413", w.Code)
	}
	if len(store.objects) != 0 {
		t.Errorf("oversized body should not be stored, got %d objects", len(store.objects))
	}

	// Valid upload → 201, stored once.
	r = httptest.NewRequest(http.MethodPost, "/screenshots", bytes.NewReader([]byte("png-bytes")))
	r.Header.Set("Content-Type", "image/png")
	w = httptest.NewRecorder()
	h(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("valid upload: got %d, want 201", w.Code)
	}
	if len(store.objects) != 1 {
		t.Errorf("valid upload should store 1 object, got %d", len(store.objects))
	}
}
