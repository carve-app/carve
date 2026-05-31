-- Track 3: reader mode + file import support

-- Store extracted/uploaded text on content items (for file imports + reader caching)
ALTER TABLE content_items
    ADD COLUMN IF NOT EXISTS body_text TEXT,
    ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'url';

-- source_type values: 'url', 'txt', 'srt', 'epub', 'anki', 'migaku', 'yomitan', 'jpdb'

-- Track import job status
CREATE TABLE IF NOT EXISTS import_jobs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type     TEXT NOT NULL,            -- 'anki', 'migaku', 'yomitan', 'jpdb'
    filename        TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',  -- pending|processing|done|error
    total_items     INT,
    imported_items  INT,
    skipped_items   INT,
    error_msg       TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ
);
