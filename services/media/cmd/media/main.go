// Media service: screenshot storage and audio proxy.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	storageDir := os.Getenv("MEDIA_STORAGE_DIR")
	if storageDir == "" {
		storageDir = "/tmp/carve-media"
	}
	if err := os.MkdirAll(filepath.Join(storageDir, "screenshots"), 0o755); err != nil {
		slog.Error("create storage dir", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "media"})
	})

	// POST /screenshots — accept PNG/JPEG, store to disk, return {id, url}.
	mux.HandleFunc("POST /screenshots", func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		var ext string
		switch {
		case strings.HasPrefix(ct, "image/png"):
			ext = ".png"
		case strings.HasPrefix(ct, "image/jpeg"):
			ext = ".jpg"
		case strings.HasPrefix(ct, "image/webp"):
			ext = ".webp"
		default:
			http.Error(w, `{"error":"unsupported content type"}`, http.StatusUnsupportedMediaType)
			return
		}

		id := fmt.Sprintf("%d", time.Now().UnixNano())
		path := filepath.Join(storageDir, "screenshots", id+ext)
		f, err := os.Create(path)
		if err != nil {
			slog.Error("create screenshot file", "error", err)
			http.Error(w, `{"error":"storage error"}`, http.StatusInternalServerError)
			return
		}
		defer f.Close()

		if _, err := io.Copy(f, io.LimitReader(r.Body, 5<<20)); err != nil {
			slog.Error("write screenshot", "error", err)
			http.Error(w, `{"error":"write error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"id":  id,
			"url": "/screenshots/" + id + ext,
		})
	})

	// GET /screenshots/{id} — serve stored screenshot.
	mux.HandleFunc("GET /screenshots/", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		// Prevent path traversal
		if strings.Contains(name, "/") || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(storageDir, "screenshots", name)
		http.ServeFile(w, r, path)
	})

	// GET /audio-proxy?url=... — proxy JapanesePod101 audio to avoid CORS.
	mux.HandleFunc("GET /audio-proxy", func(w http.ResponseWriter, r *http.Request) {
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

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(target)
		if err != nil || resp.StatusCode != http.StatusOK {
			http.Error(w, `{"error":"upstream error"}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		io.Copy(w, resp.Body)
	})

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

	slog.Info("starting media service", "addr", srv.Addr, "storage", storageDir)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
