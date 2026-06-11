-- Enforce mining idempotency: at most one live card per (user, language, lemma).
--
-- The cards.Create handler is idempotent on this triple (it returns the existing
-- card so the extension can attach media instead of creating a duplicate), but
-- that was only enforced by a best-effort SELECT-then-INSERT with a race window.
-- This partial unique index makes it authoritative: a concurrent double-mine
-- now hits a unique violation, which the handler resolves to the existing card.
--
-- Soft-deleted cards are excluded (WHERE deleted_at IS NULL) so re-mining a word
-- after deleting its card creates a fresh card, as users expect.

-- 1. Resolve any pre-existing duplicates by soft-deleting all but the oldest
--    live card in each (user, language, front_text) group. The kept row is the
--    earliest-created one (it carries the original review history).
WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY user_id, language_code, front_text
               ORDER BY created_at, id
           ) AS rn
    FROM cards
    WHERE deleted_at IS NULL
)
UPDATE cards c
   SET deleted_at = now()
  FROM ranked r
 WHERE c.id = r.id
   AND r.rn > 1;

-- 2. Enforce uniqueness going forward.
CREATE UNIQUE INDEX IF NOT EXISTS cards_user_lang_front_uniq
    ON cards (user_id, language_code, front_text)
    WHERE deleted_at IS NULL;
