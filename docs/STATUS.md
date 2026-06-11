# Carve — Current State

Source of truth for what's actually built and verified, superseding the
point-in-time `09-roadmap.md` and `10-audit.md` snapshots. The other numbered
`docs/NN-*.md` files remain accurate as design/architecture reference.

Last updated: 2026-06-11.

## Works end-to-end (verified, with tests + live checks)

- **Video sentence mining** — `m` on a subtitle → card with DRM-safe screenshot,
  exact-sentence audio (seek-to-cue, not playhead), sentence, fluent translation,
  and true cue timing. Real-Chromium full-stack e2e (`make test-video-mining`).
- **SPA-aware overlay** — mounts after client-side navigation on YouTube/Netflix/
  etc. (not just hard-loaded `/watch` URLs).
- **Word coloring + lookup popup** — status colors, definition, reading, pitch
  accent, frequency band, dictionary image (Wikipedia), word audio, AI
  explanation (Claude). Spacing preserved for Latin scripts.
- **Languages, end-to-end** (tokenize → dictionary → mine): ja, en (monolingual
  WordNet), zh-cn (CC-CEDICT), ko, es/de/fr/it/pt (FreeDict + simplemma). vi
  tokenizes only (no dictionary).
- **Translation**: Google Cloud Translation v3 Translation-LLM (single engine,
  no gloss fallback). **TTS**: Google Cloud Text-to-Speech (word + sentence).
  Both via service account (`GOOGLE_APPLICATION_CREDENTIALS`); when unset the
  feature is simply absent (no degraded fallback).
- **Review (web)**: FSRS-6, recognition/production card types, audio + image +
  translation on the card, offline review-event queue.
- **Auth**: 4h access tokens + rotating 30-day refresh, transparent refresh on
  401 in both web and extension (no mid-session "session expired").
- **Grammar** known-pattern tracking (JA, 30 JLPT patterns) with web UI.
- **Import**: Anki `.apkg`, Migaku CSV, Yomitan, JPDB. **Export**: Anki `.apkg`
  + CSV.
- **Immersion tracking**, **comprehension overlay**, **idempotent card create**,
  hardened media service (R2/local), SSRF-guarded fetches.

## Partial / known gaps

- **Native mobile apps**: none. The web app is a PWA (installable, offline
  review); there is no native iOS/Android and no mobile mining path. This is the
  single largest gap vs. Migaku.
- **Grammar** detection is JA-only (UI/persistence are language-agnostic).
- **Pitch accent** is JA-only.
- **Vietnamese** has no dictionary (coloring works, lookups return nothing).
- **FreeDict glosses** (es/de/fr/it/pt definitions) carry minor source artifacts
  (e.g. leading sense numbers); they back lookups, while *sentence translation*
  uses the v3 LLM.
- TTS/MT require a Google service account to be configured; cost/quotas apply.

## Local dev

`make setup && make import-all && make dev-seed` — see the README. The full
stack (postgres/redis + media + nlp + api + web) comes up with one command;
the extension is loaded unpacked from `apps/extension/dist/chrome`.
