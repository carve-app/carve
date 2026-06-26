package export

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

const (
	maxAPKGMediaBytes      int64 = 20 << 20
	maxAPKGMediaTotalBytes int64 = 100 << 20
)

type MediaFetcher interface {
	Fetch(context.Context, string, int64) ([]byte, string, error)
}

type mediaDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type httpMediaFetcher struct {
	serviceBase string
	publicBase  string
	client      mediaDoer
}

func newHTTPMediaFetcher() MediaFetcher {
	serviceBase := strings.TrimRight(os.Getenv("MEDIA_SERVICE_URL"), "/")
	if serviceBase == "" {
		serviceBase = "http://localhost:8002"
	}
	publicBase := strings.TrimRight(os.Getenv("MEDIA_PUBLIC_BASE"), "/")
	if publicBase == "" {
		publicBase = serviceBase
	}
	return &httpMediaFetcher{
		serviceBase: serviceBase,
		publicBase:  publicBase,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (f *httpMediaFetcher) Fetch(ctx context.Context, rawURL string, maxBytes int64) ([]byte, string, error) {
	endpoint, err := f.internalURL(rawURL)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("media GET returned %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, "", fmt.Errorf("media object exceeds %d bytes", maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxBytes {
		return nil, "", fmt.Errorf("media object exceeds %d bytes", maxBytes)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func (f *httpMediaFetcher) internalURL(rawURL string) (string, error) {
	if rawURL == f.publicBase || strings.HasPrefix(rawURL, f.publicBase+"/") {
		return f.serviceBase + strings.TrimPrefix(rawURL, f.publicBase), nil
	}
	if rawURL == f.serviceBase || strings.HasPrefix(rawURL, f.serviceBase+"/") {
		return rawURL, nil
	}
	return "", errors.New("refusing to fetch media outside configured media origin")
}

type apkgMedia struct {
	Name string
	Data []byte
}

type mediaSlot struct {
	kind    string
	url     string
	setName func(string)
}

func (h *Handler) prepareAPKGMedia(ctx context.Context, rows []exportCardRow) ([]exportCardRow, []apkgMedia, error) {
	if h.media == nil {
		h.media = newHTTPMediaFetcher()
	}
	byURL := make(map[string]string)
	var objects []apkgMedia
	var totalBytes int64
	for rowIndex := range rows {
		slots := []mediaSlot{
			{kind: "image", url: rows[rowIndex].FrontImageURL, setName: func(name string) { rows[rowIndex].FrontImageName = name }},
			{kind: "audio", url: rows[rowIndex].FrontAudioURL, setName: func(name string) { rows[rowIndex].FrontAudioName = name }},
			{kind: "back-audio", url: rows[rowIndex].BackAudioURL, setName: func(name string) { rows[rowIndex].BackAudioName = name }},
			{kind: "sentence-audio", url: rows[rowIndex].SentenceAudioURL, setName: func(name string) { rows[rowIndex].SentenceAudioName = name }},
		}
		for _, slot := range slots {
			if slot.url == "" {
				continue
			}
			if name, ok := byURL[slot.url]; ok {
				slot.setName(name)
				continue
			}
			data, contentType, err := h.media.Fetch(ctx, slot.url, maxAPKGMediaBytes)
			if err != nil {
				return nil, nil, fmt.Errorf("fetch %s: %w", slot.kind, err)
			}
			totalBytes += int64(len(data))
			if totalBytes > maxAPKGMediaTotalBytes {
				return nil, nil, fmt.Errorf("APKG media exceeds %d bytes", maxAPKGMediaTotalBytes)
			}
			ext, err := mediaExtension(slot.url, contentType, slot.kind == "image")
			if err != nil {
				return nil, nil, err
			}
			name := fmt.Sprintf("carve-%s-%03d%s", slot.kind, len(objects), ext)
			byURL[slot.url] = name
			slot.setName(name)
			objects = append(objects, apkgMedia{Name: name, Data: data})
		}
	}
	return rows, objects, nil
}

func mediaExtension(rawURL, contentType string, image bool) (string, error) {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	known := map[string]string{
		"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp",
		"audio/mpeg": ".mp3", "audio/webm": ".webm", "audio/ogg": ".ogg",
	}
	if ext := known[contentType]; ext != "" {
		return ext, nil
	}
	parsed, _ := url.Parse(rawURL)
	ext := strings.ToLower(path.Ext(parsed.Path))
	imageExt := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}
	audioExt := map[string]bool{".mp3": true, ".webm": true, ".ogg": true}
	if (image && imageExt[ext]) || (!image && audioExt[ext]) {
		return ext, nil
	}
	return "", fmt.Errorf("unsupported APKG media content type %q", contentType)
}
