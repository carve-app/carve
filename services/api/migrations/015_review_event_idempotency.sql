-- Make client retries of review submissions exactly-once.
-- Existing rows remain valid; first-party clients send client_event_id while
-- older clients continue to work without idempotency until they are upgraded.

ALTER TABLE review_events
    ADD COLUMN IF NOT EXISTS client_event_id UUID,
    ADD COLUMN IF NOT EXISTS state_after TEXT,
    ADD COLUMN IF NOT EXISTS reps_after INT,
    ADD COLUMN IF NOT EXISTS lapses_after INT,
    ADD COLUMN IF NOT EXISTS is_leech_after BOOLEAN,
    ADD COLUMN IF NOT EXISTS response_after JSONB;

CREATE UNIQUE INDEX IF NOT EXISTS review_events_client_event_idx
    ON review_events(user_id, client_event_id)
    WHERE client_event_id IS NOT NULL;
