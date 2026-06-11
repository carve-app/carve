// Package audio populates pronunciation audio for flashcards.
//
// Word audio is resolved through a per-language chain of providers
// (see providers.go). The first provider that returns a non-empty,
// validated audio URL wins; results are cached in the audio_cache table
// keyed by (language_code, lemma, provider).
//
// Sentence audio is synthesized via the TTS provider (Google Translate's
// free, key-less translate_tts endpoint) and is gated behind TTS_ENABLED.
package audio

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// populateTimeout bounds the whole background population of a single card
// (word + sentence audio combined).
const populateTimeout = 20 * time.Second

// Lookup returns a cached or freshly-resolved word-audio URL for the given
// lemma+reading in the given language. It walks the provider chain for the
// language, returning the first validated URL (and caching it). Returns ""
// when no provider can supply audio.
func Lookup(ctx context.Context, db *pgxpool.Pool, language, lemma, reading string) string {
	if lemma == "" {
		return ""
	}

	for _, p := range providersFor(language) {
		// Cache hit for this specific provider.
		if url := cachedURL(ctx, db, language, lemma, p.Name()); url != "" {
			return url
		}

		audioURL := p.WordAudio(ctx, language, lemma, reading)
		if audioURL == "" {
			continue
		}

		cacheURL(ctx, db, language, lemma, reading, p.Name(), audioURL)
		return audioURL
	}
	return ""
}

// SentenceAudio returns a cached or freshly-synthesized audio URL for a full
// sentence in the given language, using the TTS provider. Returns "" when TTS
// is disabled, the sentence is empty, or synthesis fails.
//
// The cache is keyed on the sentence text in the lemma column with a distinct
// "tts-sentence" provider so it never collides with word-audio rows.
func SentenceAudio(ctx context.Context, db *pgxpool.Pool, language, sentence string) string {
	if sentence == "" {
		return ""
	}

	tts := newTTSProvider()
	if !tts.Enabled() {
		return ""
	}
	if _, ok := ttsLangCode(language); !ok {
		return ""
	}

	const provider = "tts-sentence"
	if url := cachedURL(ctx, db, language, sentence, provider); url != "" {
		return url
	}

	audioURL := tts.synthesize(ctx, language, sentence)
	if audioURL == "" {
		return ""
	}

	cacheURL(ctx, db, language, sentence, "", provider, audioURL)
	return audioURL
}

// PopulateCard resolves and persists both word audio (front_audio_url) and
// sentence audio (sentence_audio_url) for a card. It is best-effort: each leg
// is independent, failures are logged and swallowed. Designed to run in a
// goroutine after card creation.
//
// reading may be empty for non-Japanese languages — sentence audio is still
// populated in that case.
func PopulateCard(db *pgxpool.Pool, cardID, language, lemma, reading, sentence string) {
	ctx, cancel := context.WithTimeout(context.Background(), populateTimeout)
	defer cancel()

	if wordURL := Lookup(ctx, db, language, lemma, reading); wordURL != "" {
		if _, err := db.Exec(ctx,
			`UPDATE cards SET front_audio_url = $1
			 WHERE id = $2 AND front_audio_url IS NULL`,
			wordURL, cardID,
		); err != nil {
			slog.Warn("audio populate front failed", "card_id", cardID, "error", err)
		}
	}

	if sentURL := SentenceAudio(ctx, db, language, sentence); sentURL != "" {
		if _, err := db.Exec(ctx,
			`UPDATE cards SET sentence_audio_url = $1
			 WHERE id = $2 AND sentence_audio_url IS NULL`,
			sentURL, cardID,
		); err != nil {
			slog.Warn("audio populate sentence failed", "card_id", cardID, "error", err)
		}
	}
}

// cachedURL returns a previously-cached audio URL for the given key, or "".
func cachedURL(ctx context.Context, db *pgxpool.Pool, language, lemma, provider string) string {
	var cached string
	err := db.QueryRow(ctx,
		`SELECT audio_url FROM audio_cache
		 WHERE language_code = $1 AND lemma = $2 AND provider = $3`,
		language, lemma, provider,
	).Scan(&cached)
	if err != nil {
		return ""
	}
	return cached
}

// cacheURL stores an audio URL in audio_cache. Non-fatal on failure: we simply
// re-resolve next time.
func cacheURL(ctx context.Context, db *pgxpool.Pool, language, lemma, reading, provider, audioURL string) {
	var readingArg any
	if reading != "" {
		readingArg = reading
	}
	if _, err := db.Exec(ctx,
		`INSERT INTO audio_cache (id, language_code, lemma, reading, provider, audio_url)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)
		 ON CONFLICT (language_code, lemma, provider) DO NOTHING`,
		language, lemma, readingArg, provider, audioURL,
	); err != nil {
		slog.Warn("audio cache insert failed", "provider", provider, "error", err)
	}
}
