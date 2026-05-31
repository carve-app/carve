-- Password reset tokens for the forgot-password flow.
-- Tokens are stored as SHA-256 hashes (never in plain text).
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS prt_user_idx ON password_reset_tokens (user_id);
