-- Persist scored placement attempts separately from exact per-word knowledge.
-- The estimate is diagnostic metadata; only correctly answered sample words
-- are inserted into user_word_knowledge.

CREATE TABLE placement_test_attempts (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id               UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    language_code         TEXT NOT NULL REFERENCES languages(code),
    test_version          TEXT NOT NULL,
    answers               JSONB NOT NULL,
    correct_count         INT NOT NULL CHECK (correct_count >= 0),
    total_count           INT NOT NULL CHECK (total_count > 0),
    estimated_known_words INT NOT NULL CHECK (estimated_known_words >= 0),
    estimate_lower        INT NOT NULL CHECK (estimate_lower >= 0),
    estimate_upper        INT NOT NULL CHECK (estimate_upper >= estimate_lower),
    result_label          TEXT NOT NULL,
    completed_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX placement_attempts_user_language_idx
    ON placement_test_attempts (user_id, language_code, completed_at DESC);
