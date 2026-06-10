// Media service: screenshot/audio storage and a narrow audio proxy.
//
// Uploads are persisted through the internal/storage backend (local disk in
// dev, Cloudflare R2 in prod) and addressed by a random UUID. Writes require a
// shared internal token when MEDIA_INTERNAL_TOKEN is set (the API injects it);
// reads are unauthenticated but unguessable (UUID keys) and, under R2, are
// served by Cloudflare directly rather than by this service.
package main

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/carve-app/carve/services/media/internal/storage"
	"github.com/google/uuid"
)

const (
	maxImageBytes = 5 << 20
	maxAudioBytes = 20 << 20
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	store, err := storage.New()
	if err != nil {
		slog.Error("init storage backend", "error", err)
		os.Exit(1)
	}

	// Shared secret the API presents on write requests. When empty (dev/e2e)
	// writes are open, mirroring NLP_INTERNAL_SECRET; production sets it.
	internalToken := os.Getenv("MEDIA_INTERNAL_TOKEN")
	// Used to redirect reads to Cloudflare when the backend doesn't serve bytes.
	publicBase := strings.TrimRight(os.Getenv("R2_PUBLIC_BASE"), "/")

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "media"})
	})

	writeUpload := func(prefix string, maxBytes int64, extFor func(ct string) (string, bool)) http.HandlerFunc {
		base := uploadHandler(prefix, maxBytes, extFor, store)
		return func(w http.ResponseWriter, r *http.Request) {
			if !authorizeWrite(w, r, internalToken) {
				return
			}
			base(w, r)
		}
	}

	serveHandler := func(prefix string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			name := filepath.Base(r.URL.Path)
			// Defense in depth — reject any traversal in the key.
			if name == "." || name == "/" || strings.Contains(name, "..") || strings.Contains(name, "/") {
				http.NotFound(w, r)
				return
			}
			rc, ct, err := store.Open(r.Context(), prefix, name)
			if errors.Is(err, storage.ErrUnsupported) {
				// R2 backend: bytes are served by Cloudflare, not this service.
				if publicBase != "" {
					http.Redirect(w, r, publicBase+"/"+prefix+"/"+name, http.StatusMovedPermanently)
					return
				}
				http.NotFound(w, r)
				return
			}
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer rc.Close()
			if ct != "" {
				w.Header().Set("Content-Type", ct)
			}
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			io.Copy(w, rc)
		}
	}

	mux.HandleFunc("POST /screenshots", writeUpload("screenshots", maxImageBytes, imageExt))
	mux.HandleFunc("GET /screenshots/", serveHandler("screenshots"))
	mux.HandleFunc("POST /audio", writeUpload("audio", maxAudioBytes, audioExt))
	mux.HandleFunc("GET /audio/", serveHandler("audio"))

	// GET /audio-proxy?url=... — proxy JapanesePod101 audio to avoid CORS.
	mux.HandleFunc("GET /audio-proxy", audioProxyHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8002"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("starting media service", "addr", srv.Addr, "auth", internalToken != "")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// authorizeWrite enforces the internal bearer token on write endpoints when one
// is configured. Returns true if the request may proceed.
func authorizeWrite(w http.ResponseWriter, r *http.Request, token string) bool {
	if token == "" {
		return true // dev / e2e: open writes
	}
	const prefix = "Bearer "
	got := r.Header.Get("Authorization")
	if strings.HasPrefix(got, prefix) &&
		subtle.ConstantTimeCompare([]byte(got[len(prefix):]), []byte(token)) == 1 {
		return true
	}
	http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	return false
}

// uploadHandler buffers the (size-capped) body and persists it through the
// storage backend under a random UUID key, returning {id, url}. It does NOT
// perform auth — callers wrap it with authorizeWrite.
func uploadHandler(prefix string, maxBytes int64, extFor func(ct string) (string, bool), store storage.Storage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		ext, ok := extFor(ct)
		if !ok {
			http.Error(w, `{"error":"unsupported content type"}`, http.StatusUnsupportedMediaType)
			return
		}

		// Buffer up to the cap (+1 to detect overflow). Rejecting an oversized
		// body avoids silently storing a truncated/corrupt file, and gives the
		// storage backend an exact size to upload.
		data, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
		if err != nil {
			http.Error(w, `{"error":"read error"}`, http.StatusBadRequest)
			return
		}
		if int64(len(data)) > maxBytes {
			http.Error(w, `{"error":"payload too large"}`, http.StatusRequestEntityTooLarge)
			return
		}

		key := uuid.NewString() + ext
		url, err := store.Put(r.Context(), prefix, key, ct, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			slog.Error("store object", "error", err, "prefix", prefix)
			http.Error(w, `{"error":"storage error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": key, "url": url})
	}
}

func imageExt(ct string) (string, bool) {
	switch {
	case strings.HasPrefix(ct, "image/png"):
		return ".png", true
	case strings.HasPrefix(ct, "image/jpeg"):
		return ".jpg", true
	case strings.HasPrefix(ct, "image/webp"):
		return ".webp", true
	}
	return "", false
}

func audioExt(ct string) (string, bool) {
	switch {
	case strings.HasPrefix(ct, "audio/webm"):
		return ".webm", true
	case strings.HasPrefix(ct, "audio/ogg"):
		return ".ogg", true
	case strings.HasPrefix(ct, "audio/mpeg"):
		return ".mp3", true
	}
	return "", false
}

// ── audio proxy (hardened) ────────────────────────────────────────────────────

// proxyClient refuses to follow redirects and blocks connections to non-public
// addresses at dial time, so a malicious/redirecting upstream can't turn the
// proxy into an SSRF primitive against the VPC or metadata endpoints.
var proxyClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
			Control: func(_, address string, _ syscall.RawConn) error {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return err
				}
				ip := net.ParseIP(host)
				if ip == nil || isBlockedIP(ip) {
					return errors.New("refusing to connect to a non-public address")
				}
				return nil
			},
		}).DialContext,
	},
}

func isBlockedIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	return false
}

func audioProxyHandler(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("url")
	if target == "" {
		http.Error(w, `{"error":"url required"}`, http.StatusBadRequest)
		return
	}
	// Only allow JapanesePod101 audio URLs.
	if !strings.HasPrefix(target, "https://assets.languagepod101.com/") {
		http.Error(w, `{"error":"url not allowed"}`, http.StatusForbidden)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		http.Error(w, `{"error":"bad url"}`, http.StatusBadRequest)
		return
	}
	resp, err := proxyClient.Do(req)
	if err != nil {
		http.Error(w, `{"error":"upstream error"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, `{"error":"upstream error"}`, http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.Copy(w, io.LimitReader(resp.Body, maxAudioBytes))
}
