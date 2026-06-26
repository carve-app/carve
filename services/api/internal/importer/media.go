package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	maxAnkiMediaObjectBytes int64 = 20 << 20
	maxAnkiMediaTotalBytes  int64 = 100 << 20
)

type ankiMedia struct {
	Name        string
	ContentType string
	Data        []byte
}

type MediaUploader interface {
	Upload(context.Context, string, []byte, string) (string, error)
}

type mediaDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type httpMediaUploader struct {
	serviceBase string
	publicBase  string
	token       string
	client      mediaDoer
}

func newHTTPMediaUploader() MediaUploader {
	serviceBase := strings.TrimRight(os.Getenv("MEDIA_SERVICE_URL"), "/")
	if serviceBase == "" {
		serviceBase = "http://localhost:8002"
	}
	publicBase := strings.TrimRight(os.Getenv("MEDIA_PUBLIC_BASE"), "/")
	if publicBase == "" {
		publicBase = serviceBase
	}
	return &httpMediaUploader{
		serviceBase: serviceBase,
		publicBase:  publicBase,
		token:       os.Getenv("MEDIA_INTERNAL_TOKEN"),
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *Handler) mediaUploader() MediaUploader {
	if h.media == nil {
		return newHTTPMediaUploader()
	}
	return h.media
}

func (u *httpMediaUploader) Upload(ctx context.Context, endpointPath string, data []byte, contentType string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.serviceBase+endpointPath, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)
	if u.token != "" {
		req.Header.Set("Authorization", "Bearer "+u.token)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("media service returned %d", resp.StatusCode)
	}
	var result struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&result); err != nil {
		return "", err
	}
	if result.URL == "" {
		return "", errors.New("media service returned empty URL")
	}
	if strings.HasPrefix(result.URL, "http://") || strings.HasPrefix(result.URL, "https://") {
		return result.URL, nil
	}
	return u.publicBase + "/" + strings.TrimLeft(result.URL, "/"), nil
}

func readAnkiMedia(entries map[string]*zip.File, manifest []byte) (map[string]*ankiMedia, error) {
	out := make(map[string]*ankiMedia)
	if len(manifest) == 0 {
		return out, nil
	}
	var names map[string]string
	if err := json.Unmarshal(manifest, &names); err != nil {
		return nil, fmt.Errorf("invalid Anki media manifest: %w", err)
	}
	var total int64
	for archiveName, displayName := range names {
		entry := entries[archiveName]
		if entry == nil {
			continue
		}
		if entry.UncompressedSize64 > uint64(maxAnkiMediaObjectBytes) {
			return nil, fmt.Errorf("Anki media object %q exceeds size limit", displayName)
		}
		total += int64(entry.UncompressedSize64)
		if total > maxAnkiMediaTotalBytes {
			return nil, errors.New("Anki media exceeds total size limit")
		}
		rc, err := entry.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxAnkiMediaObjectBytes+1))
		rc.Close()
		if err != nil || int64(len(data)) > maxAnkiMediaObjectBytes {
			return nil, fmt.Errorf("read Anki media object %q", displayName)
		}
		contentType, ok := mediaContentType(displayName)
		if !ok {
			continue
		}
		out[displayName] = &ankiMedia{Name: displayName, ContentType: contentType, Data: data}
	}
	return out, nil
}

func mediaContentType(name string) (string, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".webp":
		return "image/webp", true
	case ".mp3":
		return "audio/mpeg", true
	case ".webm":
		return "audio/webm", true
	case ".ogg":
		return "audio/ogg", true
	default:
		return "", false
	}
}

var (
	imageRE = regexp.MustCompile(`(?i)<img[^>]+src=["']([^"']+)["']`)
	soundRE = regexp.MustCompile(`(?i)\[sound:([^\]]+)\]`)
)

func imageName(html string) string {
	match := imageRE.FindStringSubmatch(html)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func soundNames(html string) []string {
	matches := soundRE.FindAllStringSubmatch(html, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) == 2 {
			out = append(out, match[1])
		}
	}
	return out
}

func stripSoundMarkup(value string) string {
	return soundRE.ReplaceAllString(value, "")
}
