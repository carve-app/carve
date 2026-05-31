-- Migration: 005_card_media
-- Video mining: subtitle timing and translation fields on cards.
-- (front_image_url and front_audio_url already exist from 001_initial_schema.sql)

ALTER TABLE cards
  ADD COLUMN IF NOT EXISTS video_source_url      TEXT,
  ADD COLUMN IF NOT EXISTS subtitle_start_ms     INT,
  ADD COLUMN IF NOT EXISTS subtitle_end_ms       INT,
  ADD COLUMN IF NOT EXISTS subtitle_translation  TEXT;
