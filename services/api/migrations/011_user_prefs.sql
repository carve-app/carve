-- Migration: 011_user_prefs
-- Adds preference columns to users for the weekly digest email and primary
-- target language. Both nullable so existing rows aren't affected.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS target_language TEXT,
    ADD COLUMN IF NOT EXISTS weekly_email_opt_out BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS users_weekly_optout_idx
    ON users (weekly_email_opt_out)
    WHERE weekly_email_opt_out = FALSE;
