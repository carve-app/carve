package audio

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// JapanesePod101 free audio URL pattern; does not require an API key.
	jpod101URL = "https://assets.languagepod101.com/dictionary/japanese/audiomp3/?kana=%s&kanji=%s"
	// Probe timeout — we only send HEAD to verify the URL returns audio/mpeg.
	probeTimeout = 5 * time.Second
	provider     = "jpod101"
)

var probeClient = &http.Client{Timeout: probeTimeout}

// Lookup returns a cached audio URL for the given lemma+reading, fetching
// from JapanesePod101 and caching if not already stored. Returns "" when no
// audio can be found.
func Lookup(ctx context.Context, db *pgxpool.Pool, language, lemma, reading string) string {
	if language != "ja" || reading == "" || lemma == "" {
		return ""
	}

	// Cache hit.
	var cached string
	err := db.QueryRow(ctx,
		`SELECT audio_url FROM audio_cache
		 WHERE language_code = $1 AND lemma = $2 AND provider = $3`,
		language, lemma, provider,
	).Scan(&cached)
	if err == nil {
		return cached
	}

	audioURL := fmt.Sprintf(jpod101URL, url.QueryEscape(reading), url.QueryEscape(lemma))

	// Verify the URL serves audio before caching it.
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, audioURL, nil)
	if err != nil {
		return ""
	}
	resp, err := probeClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "audio/mpeg" && ct != "audio/mp3" {
		// JapanesePod101 returns a silent MP3 with Content-Length:52 for missing entries;
		// anything shorter than ~1 kB is the "not found" placeholder.
		if resp.ContentLength > 0 && resp.ContentLength < 1024 {
			return ""
		}
	}

	// Store in cache (non-fatal if it fails — we'll re-probe next time).
	_, cacheErr := db.Exec(ctx,
		`INSERT INTO audio_cache (id, language_code, lemma, reading, provider, audio_url)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)
		 ON CONFLICT (language_code, lemma, provider) DO NOTHING`,
		language, lemma, reading, provider, audioURL,
	)
	if cacheErr != nil {
		slog.Warn("audio cache insert failed", "error", cacheErr)
	}

	return audioURL
}

// PopulateCard fetches audio for a card and writes front_audio_url if found.
// Designed to be called in a goroutine; logs errors but does not return them.
func PopulateCard(db *pgxpool.Pool, cardID, language, lemma, reading string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	audioURL := Lookup(ctx, db, language, lemma, reading)
	if audioURL == "" {
		return
	}

	_, err := db.Exec(ctx,
		`UPDATE cards SET front_audio_url = $1
		 WHERE id = $2 AND front_audio_url IS NULL`,
		audioURL, cardID,
	)
	if err != nil {
		slog.Warn("audio populate card failed", "card_id", cardID, "error", err)
	}
}
