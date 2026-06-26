# Carve — Current State

Source of truth for what's actually built and verified, superseding the
point-in-time `09-roadmap.md` and `10-audit.md` snapshots. The other numbered
`docs/NN-*.md` files remain accurate as design/architecture reference.

Last updated: 2026-06-26. See `16-full-audit-2026-06-26.md` for evidence,
severity, and live-provider outcomes.

## Works end-to-end (verified by enforced automated tests)

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
  translation on the card, durable offline review-event queue, and server-side
  exactly-once replay using client event IDs.
- **Auth**: 4h access tokens + rotating 30-day refresh, transparent refresh on
  401 in both web and extension, SMTP verification/reset delivery, and a
  real-stack Mailpit journey.
- **Grammar** known-pattern tracking (JA, 30 JLPT patterns) with web UI.
- **Import**: Anki `.apkg`, Migaku CSV, Yomitan, JPDB. **Export**: Anki `.apkg`
  + CSV.
- **Immersion tracking**, **comprehension overlay**, **idempotent card create**,
  hardened media service (R2/local), SSRF-guarded fetches.
- **Packaging**: static Cloudflare-compatible web output; Chrome runtime
  journeys; Firefox package/content-script runtime smoke; Safari bundle checks.

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
- **Public live YouTube subtitle canary failed on 2026-06-26**: the packaged
  overlay mounted, but the anonymous provider environment produced no cue and
  YouTube reported captions unavailable. Recorded fixtures and local real-video
  tests pass; this is not recorded as a live pass.
- Google Translation/TTS and Anthropic explanation were **inconclusive** in the
  audit environment because credentials were absent. Authenticated paid
  streaming providers were inconclusive because no sessions were in scope.
- Safari is build/manifest-verified only; no native wrapper/signing project
  exists, so runtime is **unverified**.

## Local dev

`make setup && make import-all && make dev-seed` — see the README. The full
stack (postgres/redis + Mailpit + media + nlp + api + web) comes up with one command;
the extension is loaded unpacked from `apps/extension/dist/chrome`.
