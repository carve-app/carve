-- Migration: 007_track4_power_review
-- Power-user review: card editing, lifecycle, search, bulk actions, undo.

ALTER TABLE cards
    ADD COLUMN IF NOT EXISTS notes TEXT,
    ADD COLUMN IF NOT EXISTS tags  TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS is_leech BOOLEAN NOT NULL DEFAULT FALSE;

-- Prior state snapshot on review_events to support POST /v1/review/undo.
ALTER TABLE review_events
    ADD COLUMN IF NOT EXISTS prior_fsrs_state      TEXT,
    ADD COLUMN IF NOT EXISTS prior_fsrs_stability  NUMERIC(10,4),
    ADD COLUMN IF NOT EXISTS prior_fsrs_difficulty NUMERIC(4,2),
    ADD COLUMN IF NOT EXISTS prior_fsrs_due        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS prior_fsrs_reps       INT,
    ADD COLUMN IF NOT EXISTS prior_fsrs_lapses     INT;

-- Full-text search index for /v1/cards?search=...
CREATE INDEX IF NOT EXISTS cards_fts_idx
    ON cards USING gin(
        to_tsvector('simple',
            coalesce(front_text, '') || ' ' ||
            coalesce(front_reading, '') || ' ' ||
            coalesce(back_text, '') || ' ' ||
            coalesce(sentence, '')
        )
    )
    WHERE deleted_at IS NULL;
