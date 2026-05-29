# Data Models

All tables use PostgreSQL. UUIDs for primary keys (v7, time-sortable). `updated_at` is auto-maintained by a trigger. Soft deletes via `deleted_at` where appropriate.

---

## Users & Auth

```sql
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT UNIQUE NOT NULL,
    display_name  TEXT NOT NULL,
    avatar_url    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);

CREATE TABLE user_auth (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,         -- 'email', 'google', 'github'
    provider_id     TEXT,                  -- NULL for email/password
    password_hash   TEXT,                  -- NULL for OAuth
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, provider_id)
);

CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,      -- SHA-256 of the raw token
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    user_agent  TEXT,
    ip_address  INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE subscriptions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tier            TEXT NOT NULL,         -- 'free', 'learner', 'pro'
    status          TEXT NOT NULL,         -- 'active', 'cancelled', 'past_due'
    provider        TEXT,                  -- 'stripe', 'self_hosted'
    provider_sub_id TEXT,                  -- Stripe subscription ID
    current_period_end TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

---

## Language Configuration

```sql
-- Supported languages (seeded data)
CREATE TABLE languages (
    code        TEXT PRIMARY KEY,          -- BCP-47: 'ja', 'zh-cn', 'ko', 'es', etc.
    name_en     TEXT NOT NULL,
    name_native TEXT NOT NULL,
    script      TEXT NOT NULL,             -- 'latin', 'cjk', 'hangul', 'arabic', etc.
    has_spaces  BOOLEAN NOT NULL,          -- False for Japanese, Chinese
    has_tones   BOOLEAN NOT NULL DEFAULT FALSE,
    rtl         BOOLEAN NOT NULL DEFAULT FALSE,
    wasm_tokenizer TEXT,                   -- path to WASM module, NULL if server-only
    enabled     BOOLEAN NOT NULL DEFAULT TRUE
);

-- Per-user language settings
CREATE TABLE user_languages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    native_language TEXT NOT NULL,         -- BCP-47, for dictionary direction
    target_retention NUMERIC(3,2) NOT NULL DEFAULT 0.90,  -- 0.80-0.95
    daily_review_limit INT,               -- NULL = unlimited
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, language_code)
);
```

---

## Vocabulary & Word Knowledge

```sql
-- The global word/token registry for each language
CREATE TABLE words (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    language_code   TEXT NOT NULL REFERENCES languages(code),
    lemma           TEXT NOT NULL,         -- dictionary form
    reading         TEXT,                  -- pronunciation (kana for JP, pinyin for ZH)
    pitch_accent    TEXT,                  -- JSON: [{mora: 0, type: 'H'}, ...]
    pos             TEXT,                  -- part of speech (noun, verb, etc.)
    frequency_rank  INT,                   -- 1 = most frequent
    jlpt_level      TEXT,                  -- 'N5'..'N1' for Japanese
    hsk_level       INT,                   -- 1..9 for Chinese
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(language_code, lemma, reading)  -- same lemma can have diff readings
);

CREATE INDEX words_language_lemma_idx ON words(language_code, lemma);
CREATE INDEX words_frequency_idx ON words(language_code, frequency_rank);

-- Definitions for each word (one word → many definitions, many languages)
CREATE TABLE word_definitions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    word_id         UUID NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    target_language TEXT NOT NULL,         -- the definition language (e.g., 'en')
    sense_index     INT NOT NULL DEFAULT 0,
    definition      TEXT NOT NULL,
    part_of_speech  TEXT,
    tags            TEXT[],                -- ['formal', 'archaic', 'colloquial', ...]
    source          TEXT NOT NULL,         -- 'jmdict', 'cedict', 'wiktionary', 'user'
    confidence      NUMERIC(3,2) NOT NULL DEFAULT 1.0
);

-- Example sentences (from Tatoeba or mined from user content)
CREATE TABLE example_sentences (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    word_id         UUID NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    text            TEXT NOT NULL,
    translation     TEXT,
    translation_lang TEXT NOT NULL DEFAULT 'en',
    audio_url       TEXT,
    source          TEXT,                  -- 'tatoeba', 'user_mined', 'generated'
    quality_score   NUMERIC(3,2),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- User's knowledge state for each word (per language)
CREATE TABLE user_word_knowledge (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    word_id         UUID NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'unknown',
    -- 'unknown', 'learning', 'known', 'mature', 'suspended', 'ignored'
    first_seen_at   TIMESTAMPTZ,
    known_since     TIMESTAMPTZ,           -- when status moved to 'known'
    review_count    INT NOT NULL DEFAULT 0,
    correct_count   INT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, word_id)
);

CREATE INDEX uwk_user_lang_idx ON user_word_knowledge(user_id)
    INCLUDE (word_id, status);
```

---

## Cards & SRS

```sql
-- A card is a single reviewable unit
CREATE TABLE cards (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deck_id         UUID REFERENCES decks(id),
    word_id         UUID REFERENCES words(id),         -- NULL for phrase cards
    language_code   TEXT NOT NULL REFERENCES languages(code),
    card_type       TEXT NOT NULL,
    -- 'recognition'  (see target word, recall meaning)
    -- 'production'   (see meaning in L1, produce target word)
    -- 'audio'        (hear word, identify meaning)
    -- 'reading'      (see written form, produce reading/pronunciation)
    -- 'cloze'        (fill in blank in sentence)

    -- Card content fields
    front_text      TEXT,
    front_audio_url TEXT,
    front_image_url TEXT,
    back_text       TEXT,
    back_audio_url  TEXT,
    sentence        TEXT,                  -- full sentence context
    sentence_audio_url TEXT,
    translation     TEXT,                  -- sentence translation

    -- Source metadata
    source_type     TEXT,                  -- 'mined', 'deck', 'generated'
    source_url      TEXT,                  -- URL where the card was mined
    source_timestamp NUMERIC,             -- video timestamp in seconds

    -- SRS state (FSRS-6 parameters)
    fsrs_stability  NUMERIC(10,4),         -- S: days until 90% retention
    fsrs_difficulty NUMERIC(4,2),          -- D: 1-10
    fsrs_due        TIMESTAMPTZ,           -- next review time
    fsrs_last_review TIMESTAMPTZ,
    fsrs_reps       INT NOT NULL DEFAULT 0,
    fsrs_lapses     INT NOT NULL DEFAULT 0,
    fsrs_state      TEXT NOT NULL DEFAULT 'new',
    -- 'new', 'learning', 'review', 'relearning'

    suspended       BOOLEAN NOT NULL DEFAULT FALSE,
    buried          BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX cards_due_idx ON cards(user_id, fsrs_due)
    WHERE deleted_at IS NULL AND suspended = FALSE AND buried = FALSE;
CREATE INDEX cards_deck_idx ON cards(deck_id) WHERE deleted_at IS NULL;

-- Every review event is immutable (event log for sync and analytics)
CREATE TABLE review_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    card_id         UUID NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reviewed_at     TIMESTAMPTZ NOT NULL,
    rating          SMALLINT NOT NULL,     -- FSRS: 1=Again, 2=Hard, 3=Good, 4=Easy
    time_taken_ms   INT,                   -- milliseconds to answer
    -- FSRS state after this review
    stability_after NUMERIC(10,4),
    difficulty_after NUMERIC(4,2),
    due_after       TIMESTAMPTZ,
    retrievability_at_review NUMERIC(4,3) -- R value when the card was shown
);

CREATE INDEX review_events_card_idx ON review_events(card_id, reviewed_at DESC);
CREATE INDEX review_events_user_idx ON review_events(user_id, reviewed_at DESC);
```

---

## Decks

```sql
CREATE TABLE decks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    name            TEXT NOT NULL,
    description     TEXT,
    is_public       BOOLEAN NOT NULL DEFAULT FALSE,
    is_official     BOOLEAN NOT NULL DEFAULT FALSE,  -- Carve-curated
    tags            TEXT[],
    cover_url       TEXT,
    card_count      INT NOT NULL DEFAULT 0,          -- denormalized
    download_count  INT NOT NULL DEFAULT 0,
    avg_rating      NUMERIC(3,2),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX decks_public_lang_idx ON decks(language_code, is_public)
    WHERE deleted_at IS NULL;

CREATE TABLE deck_ratings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deck_id         UUID NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating          SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(deck_id, user_id)
);

-- Tracks which pre-built decks a user has subscribed to
CREATE TABLE user_deck_subscriptions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deck_id         UUID NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    position        INT,                   -- display order
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, deck_id)
);
```

---

## Content & Library

```sql
-- Content items the user has saved or that have been indexed
CREATE TABLE content_items (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    language_code   TEXT NOT NULL REFERENCES languages(code),
    content_type    TEXT NOT NULL,         -- 'video', 'article', 'epub', 'subtitle'
    url             TEXT,                  -- NULL for uploaded content
    title           TEXT NOT NULL,
    thumbnail_url   TEXT,
    duration_sec    INT,                   -- for video/audio
    word_count      INT,
    -- Precomputed vocabulary profile (JSON): {frequency_band: count, ...}
    vocab_profile   JSONB,
    total_unique_words INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    indexed_at      TIMESTAMPTZ
);

CREATE INDEX content_items_lang_idx ON content_items(language_code, content_type);

-- Per-user library item (saves a content item to user's library)
CREATE TABLE user_library_items (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_id      UUID NOT NULL REFERENCES content_items(id),
    -- Comprehension at the time of saving (recalculated on demand)
    comprehension_pct NUMERIC(5,2),
    unknown_word_count INT,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    progress_pct    NUMERIC(5,2),          -- 0-100 reading/watching progress
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, content_id)
);
```

---

## Immersion Tracking

```sql
-- A single immersion session log entry
CREATE TABLE immersion_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    session_type    TEXT NOT NULL,         -- 'reading', 'listening', 'watching', 'review'
    content_id      UUID REFERENCES content_items(id),
    started_at      TIMESTAMPTZ NOT NULL,
    ended_at        TIMESTAMPTZ,
    duration_sec    INT,                   -- computed from ended_at - started_at
    words_mined     INT NOT NULL DEFAULT 0,
    lookups         INT NOT NULL DEFAULT 0,
    source          TEXT NOT NULL DEFAULT 'extension',
    -- 'extension' (auto-tracked), 'manual' (user-entered), 'mobile'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX immersion_sessions_user_date_idx
    ON immersion_sessions(user_id, started_at DESC);
CREATE INDEX immersion_sessions_lang_idx
    ON immersion_sessions(user_id, language_code, started_at DESC);
```

---

## Output Practice

```sql
CREATE TABLE output_exercises (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    exercise_type   TEXT NOT NULL,
    -- 'sentence_writing', 'cloze', 'shadowing', 'speaking'
    prompt          TEXT NOT NULL,
    target_words    UUID[],                -- word_ids that should appear in answer
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE output_attempts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    exercise_id     UUID NOT NULL REFERENCES output_exercises(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    input_text      TEXT,                  -- user's written answer
    input_audio_url TEXT,                  -- user's audio recording
    ai_feedback     TEXT,                  -- AI correction/feedback JSON
    score           NUMERIC(3,2),          -- 0-1 quality score
    attempted_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

---

## Denormalization & Materialized Views

For performance on the stats dashboard, maintain these materialized views:

```sql
-- Daily immersion summary (refreshed hourly)
CREATE MATERIALIZED VIEW daily_immersion_summary AS
SELECT
    user_id,
    language_code,
    DATE(started_at AT TIME ZONE 'UTC') AS day,
    SUM(duration_sec) FILTER (WHERE session_type = 'reading') AS reading_sec,
    SUM(duration_sec) FILTER (WHERE session_type IN ('listening','watching')) AS listening_sec,
    SUM(duration_sec) FILTER (WHERE session_type = 'review') AS review_sec,
    SUM(words_mined) AS words_mined,
    SUM(lookups) AS lookups
FROM immersion_sessions
GROUP BY user_id, language_code, DATE(started_at AT TIME ZONE 'UTC')
WITH DATA;

CREATE UNIQUE INDEX ON daily_immersion_summary(user_id, language_code, day);

-- Known word count over time (append-only snapshot, taken daily)
CREATE TABLE word_count_snapshots (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    snapshotted_at  DATE NOT NULL,
    known_count     INT NOT NULL,          -- status IN ('known','mature')
    mature_count    INT NOT NULL,          -- status = 'mature'
    learning_count  INT NOT NULL,
    UNIQUE(user_id, language_code, snapshotted_at)
);
```
