-- Migration: 003_audio_reading
-- Add front_reading to cards (used for audio lookup), and subscription index.

ALTER TABLE cards ADD COLUMN IF NOT EXISTS front_reading TEXT;

CREATE INDEX IF NOT EXISTS cards_user_lang_reading_idx
    ON cards(user_id, language_code, front_reading)
    WHERE front_reading IS NOT NULL AND front_audio_url IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS subscriptions_user_idx
    ON subscriptions(user_id);
