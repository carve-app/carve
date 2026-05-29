-- Migration: 001_initial_schema
-- Creates all core tables for Phase 0.
-- Up-only (no down migration for initial schema).

-- ── Extensions ────────────────────────────────────────────────────────────────

CREATE EXTENSION IF NOT EXISTS "pgcrypto";  -- gen_random_uuid()

-- ── Helpers: auto-update updated_at ──────────────────────────────────────────

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ── Users & Auth ──────────────────────────────────────────────────────────────

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT UNIQUE NOT NULL,
    display_name  TEXT NOT NULL,
    avatar_url    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);

CREATE TRIGGER users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE user_auth (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,
    provider_id     TEXT,
    password_hash   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, provider_id) DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX user_auth_user_id_idx ON user_auth(user_id);

CREATE TABLE refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    user_agent  TEXT,
    ip_address  INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX refresh_tokens_user_idx ON refresh_tokens(user_id);
CREATE INDEX refresh_tokens_hash_idx ON refresh_tokens(token_hash);

CREATE TABLE subscriptions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tier                TEXT NOT NULL DEFAULT 'free',
    status              TEXT NOT NULL DEFAULT 'active',
    provider            TEXT,
    provider_sub_id     TEXT,
    current_period_end  TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ── Language Configuration ────────────────────────────────────────────────────

CREATE TABLE languages (
    code            TEXT PRIMARY KEY,
    name_en         TEXT NOT NULL,
    name_native     TEXT NOT NULL,
    script          TEXT NOT NULL,
    has_spaces      BOOLEAN NOT NULL DEFAULT TRUE,
    has_tones       BOOLEAN NOT NULL DEFAULT FALSE,
    rtl             BOOLEAN NOT NULL DEFAULT FALSE,
    wasm_tokenizer  TEXT,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE
);

INSERT INTO languages (code, name_en, name_native, script, has_spaces, has_tones, wasm_tokenizer) VALUES
    ('ja',    'Japanese',           '日本語',   'cjk',     FALSE, FALSE, '/wasm/ja-tokenizer.wasm'),
    ('zh-cn', 'Chinese (Simplified)', '中文',  'cjk',     FALSE, TRUE,  '/wasm/zh-tokenizer.wasm'),
    ('zh-tw', 'Chinese (Traditional)', '中文', 'cjk',     FALSE, TRUE,  '/wasm/zh-tokenizer.wasm'),
    ('ko',    'Korean',             '한국어',   'hangul',  TRUE,  FALSE, '/wasm/ko-tokenizer.wasm'),
    ('es',    'Spanish',            'Español',  'latin',   TRUE,  FALSE, '/wasm/latin-tokenizer.wasm'),
    ('de',    'German',             'Deutsch',  'latin',   TRUE,  FALSE, '/wasm/latin-tokenizer.wasm'),
    ('fr',    'French',             'Français', 'latin',   TRUE,  FALSE, '/wasm/latin-tokenizer.wasm'),
    ('pt',    'Portuguese',         'Português','latin',   TRUE,  FALSE, '/wasm/latin-tokenizer.wasm'),
    ('it',    'Italian',            'Italiano', 'latin',   TRUE,  FALSE, '/wasm/latin-tokenizer.wasm'),
    ('vi',    'Vietnamese',         'Tiếng Việt','latin',  TRUE,  TRUE,  NULL),
    ('en',    'English',            'English',  'latin',   TRUE,  FALSE, '/wasm/latin-tokenizer.wasm');

CREATE TABLE user_languages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    native_language TEXT NOT NULL DEFAULT 'en',
    target_retention NUMERIC(3,2) NOT NULL DEFAULT 0.90,
    daily_review_limit INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, language_code)
);

-- ── Vocabulary / Words ────────────────────────────────────────────────────────

CREATE TABLE words (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    language_code   TEXT NOT NULL REFERENCES languages(code),
    lemma           TEXT NOT NULL,
    reading         TEXT,
    pitch_accent    JSONB,
    pos             TEXT,
    frequency_rank  INT,
    jlpt_level      TEXT,
    hsk_level       INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(language_code, lemma, reading)
);

CREATE INDEX words_language_lemma_idx ON words(language_code, lemma);
CREATE INDEX words_frequency_idx ON words(language_code, frequency_rank);

CREATE TABLE word_definitions (
    id              BIGSERIAL PRIMARY KEY,
    word_id         UUID NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    target_language TEXT NOT NULL DEFAULT 'en',
    sense_index     INT NOT NULL DEFAULT 0,
    definition      TEXT NOT NULL,
    part_of_speech  TEXT,
    tags            TEXT[],
    source          TEXT NOT NULL DEFAULT 'jmdict',
    confidence      NUMERIC(3,2) NOT NULL DEFAULT 1.0
);

CREATE INDEX wdef_word_target_idx ON word_definitions(word_id, target_language);

CREATE TABLE example_sentences (
    id              BIGSERIAL PRIMARY KEY,
    word_id         UUID REFERENCES words(id) ON DELETE CASCADE,
    text            TEXT NOT NULL,
    translation     TEXT,
    translation_lang TEXT NOT NULL DEFAULT 'en',
    audio_url       TEXT,
    source          TEXT,
    quality_score   NUMERIC(3,2),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_word_knowledge (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    word_id         UUID NOT NULL REFERENCES words(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'unknown',
    first_seen_at   TIMESTAMPTZ,
    known_since     TIMESTAMPTZ,
    review_count    INT NOT NULL DEFAULT 0,
    correct_count   INT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, word_id)
);

CREATE INDEX uwk_user_lang_idx ON user_word_knowledge(user_id);

-- ── Cards & SRS ───────────────────────────────────────────────────────────────

CREATE TABLE decks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    name            TEXT NOT NULL,
    description     TEXT,
    is_public       BOOLEAN NOT NULL DEFAULT FALSE,
    is_official     BOOLEAN NOT NULL DEFAULT FALSE,
    tags            TEXT[],
    cover_url       TEXT,
    card_count      INT NOT NULL DEFAULT 0,
    download_count  INT NOT NULL DEFAULT 0,
    avg_rating      NUMERIC(3,2),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX decks_public_lang_idx ON decks(language_code, is_public)
    WHERE deleted_at IS NULL;

CREATE TABLE cards (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deck_id         UUID REFERENCES decks(id),
    word_id         UUID REFERENCES words(id),
    language_code   TEXT NOT NULL REFERENCES languages(code),
    card_type       TEXT NOT NULL DEFAULT 'recognition',

    front_text      TEXT,
    front_audio_url TEXT,
    front_image_url TEXT,
    back_text       TEXT,
    back_audio_url  TEXT,
    sentence        TEXT,
    sentence_audio_url TEXT,
    translation     TEXT,

    source_type     TEXT,
    source_url      TEXT,
    source_timestamp NUMERIC,

    -- FSRS-6 state
    fsrs_stability  NUMERIC(10,4),
    fsrs_difficulty NUMERIC(4,2),
    fsrs_due        TIMESTAMPTZ,
    fsrs_last_review TIMESTAMPTZ,
    fsrs_reps       INT NOT NULL DEFAULT 0,
    fsrs_lapses     INT NOT NULL DEFAULT 0,
    fsrs_state      TEXT NOT NULL DEFAULT 'new',

    suspended       BOOLEAN NOT NULL DEFAULT FALSE,
    buried          BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX cards_due_idx ON cards(user_id, fsrs_due)
    WHERE deleted_at IS NULL AND suspended = FALSE AND buried = FALSE;
CREATE INDEX cards_deck_idx ON cards(deck_id) WHERE deleted_at IS NULL;

CREATE TRIGGER cards_updated_at
    BEFORE UPDATE ON cards
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE review_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    card_id         UUID NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reviewed_at     TIMESTAMPTZ NOT NULL,
    rating          SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 4),
    time_taken_ms   INT,
    stability_after NUMERIC(10,4),
    difficulty_after NUMERIC(4,2),
    due_after       TIMESTAMPTZ,
    retrievability_at_review NUMERIC(4,3)
);

CREATE INDEX review_events_card_idx ON review_events(card_id, reviewed_at DESC);
CREATE INDEX review_events_user_idx ON review_events(user_id, reviewed_at DESC);

-- ── Content & Library ─────────────────────────────────────────────────────────

CREATE TABLE content_items (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    language_code   TEXT NOT NULL REFERENCES languages(code),
    content_type    TEXT NOT NULL,
    url             TEXT,
    title           TEXT NOT NULL,
    thumbnail_url   TEXT,
    duration_sec    INT,
    word_count      INT,
    vocab_profile   JSONB,
    total_unique_words INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    indexed_at      TIMESTAMPTZ
);

CREATE INDEX content_items_lang_idx ON content_items(language_code, content_type);

CREATE TABLE user_library_items (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_id      UUID NOT NULL REFERENCES content_items(id),
    comprehension_pct NUMERIC(5,2),
    unknown_word_count INT,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    progress_pct    NUMERIC(5,2),
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, content_id)
);

-- ── Immersion Tracking ────────────────────────────────────────────────────────

CREATE TABLE immersion_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    session_type    TEXT NOT NULL,
    content_id      UUID REFERENCES content_items(id),
    started_at      TIMESTAMPTZ NOT NULL,
    ended_at        TIMESTAMPTZ,
    duration_sec    INT,
    words_mined     INT NOT NULL DEFAULT 0,
    lookups         INT NOT NULL DEFAULT 0,
    source          TEXT NOT NULL DEFAULT 'extension',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX immersion_sessions_user_date_idx
    ON immersion_sessions(user_id, started_at DESC);

-- ── Word count snapshots (analytics) ─────────────────────────────────────────

CREATE TABLE word_count_snapshots (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    snapshotted_at  DATE NOT NULL,
    known_count     INT NOT NULL,
    mature_count    INT NOT NULL,
    learning_count  INT NOT NULL,
    UNIQUE(user_id, language_code, snapshotted_at)
);

-- ── Deck ratings ──────────────────────────────────────────────────────────────

CREATE TABLE deck_ratings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deck_id         UUID NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating          SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(deck_id, user_id)
);

CREATE TABLE user_deck_subscriptions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deck_id         UUID NOT NULL REFERENCES decks(id) ON DELETE CASCADE,
    position        INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, deck_id)
);
