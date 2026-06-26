-- Make "bury until tomorrow" true in storage instead of permanently hiding a
-- card. Existing buried rows receive the same next-day expiry.

ALTER TABLE cards
    ADD COLUMN IF NOT EXISTS buried_until DATE;

UPDATE cards
SET buried_until = CURRENT_DATE + 1
WHERE buried = TRUE AND buried_until IS NULL;

