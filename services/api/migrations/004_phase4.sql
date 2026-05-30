-- Migration: 004_phase4
-- Output practice tables for Phase 4 (writing, cloze, shadowing, speaking).

-- ── Output exercise definitions ───────────────────────────────────────────────
-- One row per exercise instance shown to a user. Generated server-side from
-- recently mined cards; surfaced via GET /v1/output/exercises.

CREATE TABLE output_exercises (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    card_id         UUID REFERENCES cards(id) ON DELETE SET NULL,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    exercise_type   TEXT NOT NULL,   -- 'writing' | 'cloze' | 'shadowing' | 'speaking'
    prompt          TEXT NOT NULL,   -- instruction shown to user
    target_word     TEXT NOT NULL,   -- word the exercise focuses on
    sentence        TEXT,            -- source sentence (for cloze / shadowing)
    audio_url       TEXT,            -- for shadowing/speaking drills
    expected_answer TEXT,            -- null for open-ended writing
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL DEFAULT now() + INTERVAL '24 hours'
);

CREATE INDEX output_exercises_user_lang_idx
    ON output_exercises(user_id, language_code, created_at DESC)
    WHERE expires_at > now();

-- ── Output submissions ────────────────────────────────────────────────────────
-- User's written/spoken answers plus AI feedback.

CREATE TABLE output_submissions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    exercise_id     UUID NOT NULL REFERENCES output_exercises(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    answer_text     TEXT,            -- written or transcribed answer
    answer_audio_url TEXT,           -- for speaking submissions (stored in media svc)
    score           NUMERIC(4,2),    -- 0–100 AI-assigned score
    feedback        TEXT,            -- AI feedback text
    feedback_detail JSONB,           -- structured feedback {grammar, vocab, naturalness}
    submitted_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX output_submissions_user_idx
    ON output_submissions(user_id, submitted_at DESC);

-- ── Shadowing queue ───────────────────────────────────────────────────────────
-- Tracks which audio items are in the user's shadowing queue and their status.

CREATE TABLE shadowing_queue (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    card_id         UUID REFERENCES cards(id) ON DELETE CASCADE,
    language_code   TEXT NOT NULL REFERENCES languages(code),
    audio_url       TEXT NOT NULL,
    transcript      TEXT,
    position        INT NOT NULL DEFAULT 0,
    times_shadowed  INT NOT NULL DEFAULT 0,
    last_shadowed_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, audio_url)
);

CREATE INDEX shadowing_queue_user_pos_idx
    ON shadowing_queue(user_id, language_code, position);
