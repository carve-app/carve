-- Migration: 002_phase2
-- Phase 2: FSRS params, leech tracking, JLPT seed decks, billing.

-- ── Per-user FSRS parameters ──────────────────────────────────────────────────

CREATE TABLE user_fsrs_params (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    -- 19 FSRS-6 weights stored as JSON array
    weights         JSONB NOT NULL DEFAULT '[0.4072,1.1829,3.1262,15.4722,7.2102,0.5316,1.0651,0.1544,1.0648,0.1400,0.9988,1.9813,0.1100,0.2900,2.2700,0.1000,2.9898,0.5100,0.4300]',
    target_retention NUMERIC(3,2) NOT NULL DEFAULT 0.90,
    leech_threshold  INT NOT NULL DEFAULT 5,
    daily_new_limit  INT NOT NULL DEFAULT 20,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, language_code)
);

CREATE INDEX user_fsrs_params_user_idx ON user_fsrs_params(user_id);

-- ── Notifications table for leech alerts ──────────────────────────────────────

CREATE TABLE user_notifications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,   -- 'leech_suspended', 'streak_broken', etc.
    payload     JSONB NOT NULL DEFAULT '{}',
    read_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX user_notifications_user_unread_idx
    ON user_notifications(user_id, created_at DESC)
    WHERE read_at IS NULL;

-- ── Audio cache for cards ──────────────────────────────────────────────────────

CREATE TABLE audio_cache (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    language_code TEXT NOT NULL REFERENCES languages(code),
    lemma       TEXT NOT NULL,
    reading     TEXT,
    provider    TEXT NOT NULL DEFAULT 'tts',
    audio_url   TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(language_code, lemma, provider)
);

CREATE INDEX audio_cache_lemma_idx ON audio_cache(language_code, lemma);

-- ── Screenshot / media attachments ───────────────────────────────────────────

CREATE TABLE card_attachments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    card_id     UUID NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        TEXT NOT NULL DEFAULT 'screenshot',  -- 'screenshot' | 'audio'
    storage_key TEXT NOT NULL,
    url         TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX card_attachments_card_idx ON card_attachments(card_id);

-- ── Billing: Stripe webhook events log ────────────────────────────────────────

CREATE TABLE billing_events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID REFERENCES users(id) ON DELETE SET NULL,
    stripe_event_id TEXT UNIQUE NOT NULL,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL DEFAULT '{}',
    processed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX billing_events_user_idx ON billing_events(user_id);

-- ── System user for official deck cards ──────────────────────────────────────
INSERT INTO users (id, email, display_name)
VALUES ('00000000-0000-0000-0000-000000000000', 'system@carve.internal', 'Carve System')
ON CONFLICT DO NOTHING;

-- ── Pre-built JLPT N5 deck ────────────────────────────────────────────────────
-- Seeded as public official deck; cards use front_text + back_text only.

WITH n5_deck AS (
    INSERT INTO decks (language_code, name, description, is_public, is_official, tags)
    VALUES ('ja', 'JLPT N5 Core Vocabulary', 'Essential 100 words for JLPT N5 level', TRUE, TRUE, ARRAY['jlpt', 'n5', 'beginner'])
    RETURNING id
),
n5_cards(front, back) AS (
    VALUES
        ('日本語', 'Japanese language'),
        ('私', 'I, me'),
        ('あなた', 'you'),
        ('彼', 'he, him'),
        ('彼女', 'she, her'),
        ('今日', 'today'),
        ('明日', 'tomorrow'),
        ('昨日', 'yesterday'),
        ('今', 'now'),
        ('水', 'water'),
        ('食べる', 'to eat'),
        ('飲む', 'to drink'),
        ('行く', 'to go'),
        ('来る', 'to come'),
        ('見る', 'to see, to watch'),
        ('聞く', 'to listen, to hear'),
        ('話す', 'to speak, to talk'),
        ('読む', 'to read'),
        ('書く', 'to write'),
        ('買う', 'to buy'),
        ('大きい', 'big, large'),
        ('小さい', 'small, little'),
        ('新しい', 'new'),
        ('古い', 'old'),
        ('高い', 'tall, expensive'),
        ('安い', 'cheap, inexpensive'),
        ('好き', 'like, fond of'),
        ('嫌い', 'dislike'),
        ('電車', 'train'),
        ('車', 'car'),
        ('家', 'house, home'),
        ('学校', 'school'),
        ('先生', 'teacher'),
        ('学生', 'student'),
        ('友達', 'friend'),
        ('時間', 'time'),
        ('何', 'what'),
        ('誰', 'who'),
        ('どこ', 'where'),
        ('いつ', 'when'),
        ('なぜ', 'why'),
        ('どう', 'how'),
        ('本', 'book'),
        ('映画', 'movie'),
        ('音楽', 'music'),
        ('食べ物', 'food'),
        ('飲み物', 'drink, beverage'),
        ('名前', 'name'),
        ('国', 'country'),
        ('仕事', 'work, job')
)
INSERT INTO cards (user_id, deck_id, language_code, front_text, back_text, fsrs_state)
SELECT
    '00000000-0000-0000-0000-000000000000'::uuid,
    n5_deck.id,
    'ja',
    n5_cards.front,
    n5_cards.back,
    'new'
FROM n5_deck, n5_cards
ON CONFLICT DO NOTHING;

-- Update deck card count
UPDATE decks SET card_count = (
    SELECT COUNT(*) FROM cards WHERE deck_id = decks.id AND deleted_at IS NULL
)
WHERE name = 'JLPT N5 Core Vocabulary';

-- ── Pre-built JLPT N4 deck ────────────────────────────────────────────────────

WITH n4_deck AS (
    INSERT INTO decks (language_code, name, description, is_public, is_official, tags)
    VALUES ('ja', 'JLPT N4 Core Vocabulary', 'Essential vocabulary for JLPT N4 level', TRUE, TRUE, ARRAY['jlpt', 'n4', 'elementary'])
    RETURNING id
),
n4_cards(front, back) AS (
    VALUES
        ('働く', 'to work'),
        ('始める', 'to start, to begin'),
        ('終わる', 'to end, to finish'),
        ('使う', 'to use'),
        ('持つ', 'to hold, to have'),
        ('考える', 'to think, to consider'),
        ('知る', 'to know'),
        ('覚える', 'to remember, to memorize'),
        ('忘れる', 'to forget'),
        ('教える', 'to teach, to tell'),
        ('難しい', 'difficult'),
        ('易しい', 'easy'),
        ('楽しい', 'fun, enjoyable'),
        ('悲しい', 'sad'),
        ('嬉しい', 'happy, glad'),
        ('丁寧', 'polite, careful'),
        ('元気', 'healthy, energetic'),
        ('大切', 'important, precious'),
        ('有名', 'famous'),
        ('便利', 'convenient'),
        ('会社', 'company, workplace'),
        ('銀行', 'bank'),
        ('病院', 'hospital'),
        ('図書館', 'library'),
        ('駅', 'station'),
        ('空港', 'airport'),
        ('旅行', 'travel'),
        ('料理', 'cooking, cuisine'),
        ('天気', 'weather'),
        ('季節', 'season'),
        ('春', 'spring'),
        ('夏', 'summer'),
        ('秋', 'autumn, fall'),
        ('冬', 'winter'),
        ('体', 'body'),
        ('頭', 'head'),
        ('目', 'eye'),
        ('耳', 'ear'),
        ('口', 'mouth'),
        ('手', 'hand'),
        ('足', 'foot, leg'),
        ('声', 'voice'),
        ('言葉', 'word, language'),
        ('文化', 'culture'),
        ('歴史', 'history'),
        ('社会', 'society'),
        ('経済', 'economy'),
        ('政治', 'politics'),
        ('問題', 'problem, issue'),
        ('答え', 'answer')
)
INSERT INTO cards (user_id, deck_id, language_code, front_text, back_text, fsrs_state)
SELECT
    '00000000-0000-0000-0000-000000000000'::uuid,
    n4_deck.id,
    'ja',
    n4_cards.front,
    n4_cards.back,
    'new'
FROM n4_deck, n4_cards
ON CONFLICT DO NOTHING;

UPDATE decks SET card_count = (
    SELECT COUNT(*) FROM cards WHERE deck_id = decks.id AND deleted_at IS NULL
)
WHERE name = 'JLPT N4 Core Vocabulary';
