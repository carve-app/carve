-- Stores the AnkiConnect note ID after a Carve card has been pushed,
-- so re-running the sync never duplicates the card.

ALTER TABLE cards
    ADD COLUMN IF NOT EXISTS external_anki_id BIGINT;

CREATE INDEX IF NOT EXISTS cards_external_anki_idx
    ON cards (user_id, external_anki_id)
    WHERE external_anki_id IS NOT NULL;
