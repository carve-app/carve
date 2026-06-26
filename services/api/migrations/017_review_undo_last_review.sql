-- Preserve the complete pre-review scheduling snapshot so Undo can restore a
-- card exactly. Migration 007 stored the prior due date but not the prior last
-- review timestamp.

ALTER TABLE review_events
    ADD COLUMN IF NOT EXISTS prior_fsrs_last_review TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS prior_suspended BOOLEAN,
    ADD COLUMN IF NOT EXISTS prior_is_leech BOOLEAN;
