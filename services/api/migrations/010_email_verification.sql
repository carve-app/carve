-- Email verification tokens.
-- Tokens are stored as SHA-256 hashes (never in plain text). When a user
-- clicks the link in their email, the API hashes the supplied token and
-- looks it up; success flips users.email_verified_at and burns the token.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS evt_user_idx ON email_verification_tokens (user_id);
