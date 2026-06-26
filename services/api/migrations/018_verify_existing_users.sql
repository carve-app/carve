-- Email verification was previously advisory: registration issued a session
-- immediately and login did not enforce users.email_verified_at. Preserve
-- access for accounts created under that behavior before enforcement begins;
-- users registered after this migration remain unverified until they use the
-- single-use emailed token.

UPDATE users
SET email_verified_at = COALESCE(email_verified_at, created_at)
WHERE email_verified_at IS NULL;
