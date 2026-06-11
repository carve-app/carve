-- Migration: 014_grammar_patterns
-- Persists the grammar patterns a user has marked as "known". The NLP service
-- detects JLPT grammar patterns (see services/nlp/src/grammar_ja.py) with stable
-- string ids; this table records which of those ids each user already knows so
-- the web UI and tokenizer can compute grammar comprehension.

CREATE TABLE IF NOT EXISTS user_known_patterns (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    language_code TEXT NOT NULL,
    pattern_id    TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, language_code, pattern_id)
);

CREATE INDEX IF NOT EXISTS idx_user_known_patterns_user_lang
    ON user_known_patterns (user_id, language_code);
