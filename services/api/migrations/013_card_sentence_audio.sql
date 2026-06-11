-- Migration: 013_card_sentence_audio
-- Multi-language word + sentence audio for flashcards.
--
-- The cards.sentence_audio_url column already exists (from 001) but was never
-- populated; the audio package now fills it (and front_audio_url) in the
-- background after card creation. No new column is required.
--
-- Add a partial index so the background populator can cheaply find cards still
-- missing word audio for a given language. Idempotent: safe to re-run.

CREATE INDEX IF NOT EXISTS cards_audio_pending
    ON cards(language_code)
    WHERE front_audio_url IS NULL;
