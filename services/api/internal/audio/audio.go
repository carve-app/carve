// Package audio populates pronunciation audio for flashcards.
//
// Word and sentence audio are synthesized by Google Cloud Text-to-Speech
// (see providers.go), authenticated with a service account. There is no
// lower-quality fallback: when the engine is not configured, audio is absent.
// Results are cached in the audio_cache table keyed by
// (language_code, lemma, provider).
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

// provider is the single TTS engine (Google Cloud TTS via service account).
const ttsCacheKey = "google_cloud_tts"

// Synthesizer is the TTS provider boundary. Production uses Google Cloud;
// tests can inject deterministic timeout, malformed-response, and retry
// behavior without credentials or network access.
type Synthesizer interface {
	WordAudio(context.Context, string, string, string) string
	Synthesize(context.Context, string, string) string
}

// Lookup returns a cached or freshly-synthesized word-audio URL for the given
// lemma in the given language. Returns "" when the engine is unconfigured, the
// language is unsupported, or synthesis fails. (`reading` is unused — Cloud TTS
// pronounces the lemma directly.)
func Lookup(ctx context.Context, db *pgxpool.Pool, language, lemma, reading string) string {
	return LookupWithSynthesizer(ctx, db, newGoogleTTSProvider(), language, lemma, reading)
}

func LookupWithSynthesizer(ctx context.Context, db *pgxpool.Pool, synth Synthesizer, language, lemma, reading string) string {
	if lemma == "" {
		return ""
	}
	if url := cachedURL(ctx, db, language, lemma, ttsCacheKey); url != "" {
		return url
	}
	audioURL := synth.WordAudio(ctx, language, lemma, reading)
	if audioURL == "" {
		return ""
	}
	cacheURL(ctx, db, language, lemma, reading, ttsCacheKey, audioURL)
	return audioURL
}

// SentenceAudio returns a cached or freshly-synthesized audio URL for a full
// sentence. Returns "" when the engine is unconfigured, the language is
// unsupported, or synthesis fails.
//
// The cache is keyed on the sentence text in the lemma column with a distinct
// provider key so it never collides with word-audio rows.
func SentenceAudio(ctx context.Context, db *pgxpool.Pool, language, sentence string) string {
	return SentenceAudioWithSynthesizer(ctx, db, newGoogleTTSProvider(), language, sentence)
}

func SentenceAudioWithSynthesizer(ctx context.Context, db *pgxpool.Pool, synth Synthesizer, language, sentence string) string {
	if sentence == "" {
		return ""
	}
	if _, ok := cloudTTSLangCode(language); !ok {
		return ""
	}

	const provider = "google_cloud_tts-sentence"
	if url := cachedURL(ctx, db, language, sentence, provider); url != "" {
		return url
	}
	audioURL := synth.Synthesize(ctx, language, sentence)
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
