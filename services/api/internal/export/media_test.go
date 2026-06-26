package export

import (
	"context"
	"errors"
	"testing"
)

type mediaFetcherFunc func(context.Context, string, int64) ([]byte, string, error)

func (f mediaFetcherFunc) Fetch(ctx context.Context, url string, maxBytes int64) ([]byte, string, error) {
	return f(ctx, url, maxBytes)
}

func TestPrepareAPKGMediaFetchesAndDeduplicates(t *testing.T) {
	calls := 0
	h := &Handler{media: mediaFetcherFunc(func(_ context.Context, url string, maxBytes int64) ([]byte, string, error) {
		calls++
		if maxBytes != maxAPKGMediaBytes {
			t.Fatalf("unexpected limit: %d", maxBytes)
		}
		switch url {
		case "https://media.test/shared.png":
			return []byte("image"), "image/png", nil
		case "https://media.test/audio.webm":
			return []byte("audio"), "audio/webm;codecs=opus", nil
		default:
			return nil, "", errors.New("unexpected URL")
		}
	})}
	rows, objects, err := h.prepareAPKGMedia(context.Background(), []exportCardRow{
		{FrontImageURL: "https://media.test/shared.png", FrontAudioURL: "https://media.test/audio.webm"},
		{FrontImageURL: "https://media.test/shared.png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(objects) != 2 {
		t.Fatalf("calls=%d objects=%d, want 2/2", calls, len(objects))
	}
	if rows[0].FrontImageName == "" || rows[0].FrontImageName != rows[1].FrontImageName {
		t.Fatalf("shared image not deduplicated: %#v", rows)
	}
	if rows[0].FrontAudioName == "" {
		t.Fatal("audio archive name missing")
	}
}

func TestHTTPMediaFetcherRejectsUnknownOrigin(t *testing.T) {
	f := &httpMediaFetcher{serviceBase: "http://media:8002", publicBase: "https://cdn.test"}
	if _, err := f.internalURL("http://169.254.169.254/latest/meta-data"); err == nil {
		t.Fatal("expected unknown origin to be rejected")
	}
}

func TestPrepareAPKGMediaRejectsOversizedTotal(t *testing.T) {
	data := make([]byte, maxAPKGMediaBytes)
	h := &Handler{media: mediaFetcherFunc(func(_ context.Context, _ string, _ int64) ([]byte, string, error) {
		return data, "audio/webm", nil
	})}
	rows := make([]exportCardRow, 6)
	for i := range rows {
		rows[i].FrontAudioURL = string(rune('a' + i))
	}
	if _, _, err := h.prepareAPKGMedia(context.Background(), rows); err == nil {
		t.Fatal("expected total APKG media limit to reject six 20 MiB objects")
	}
}
