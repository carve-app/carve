-- Migration: 010_discover_articles
-- Stores ingested third-party reading material (initially NHK Easy News) so
-- the discover endpoint can rank by per-user comprehension % without
-- re-tokenising on every request.

CREATE TABLE IF NOT EXISTS discover_articles (
    id                    TEXT PRIMARY KEY,
    source                TEXT NOT NULL,
    language_code         TEXT NOT NULL,
    title                 TEXT NOT NULL,
    summary               TEXT,
    body                  TEXT NOT NULL,
    url                   TEXT NOT NULL UNIQUE,
    content_lemmas        TEXT[] NOT NULL DEFAULT '{}',
    total_content_words   INT NOT NULL DEFAULT 0,
    published_at          TIMESTAMPTZ,
    fetched_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_discover_lang_published
    ON discover_articles (language_code, published_at DESC NULLS LAST);

CREATE INDEX IF NOT EXISTS idx_discover_source
    ON discover_articles (source);
