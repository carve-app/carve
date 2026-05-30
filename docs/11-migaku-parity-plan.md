# Migaku-Parity Plan

> **Goal:** ship a drop-in replacement for Migaku — same workflows, but with a
> fresh design system, a more accurate tokenizer, and a more reliable runtime.
>
> **Scope:** everything between current state (see `10-audit.md`) and a
> public-beta-ready product. Native mobile (Phase 5 of `09-roadmap.md`) and
> intelligence layer (Phase 6) remain out of scope here.
>
> **Sequencing principle:** ship features in the order a real user encounters
> them. Landing → install → first card → first review → daily loop → power
> features. A track is "done" when the user can complete that step without
> reading docs.

---

## Track 0 — Foundation cleanup (1–2 weeks)

Prerequisite for everything else. No user-visible feature here; this is paving.

### 0.1 Shared app shell
- Create `apps/web/src/routes/+layout.svelte` with:
  - persistent sidebar (Home, Review, Library, Decks, Stats, Output, Settings)
  - top bar with language switcher, due-count badge, notifications bell,
    user menu
  - global toast region (`<Toast />`)
  - auth guard: redirects to `/login` if no token; otherwise calls
    `/v1/users/me` once on load and stores in a context store
- Delete the per-page `<header><nav>` blocks from every existing route.
- Add a Svelte store `$lib/stores/lang.ts` that reflects the selected
  language and persists to `localStorage`. Every page reads from this instead
  of hardcoding `'ja'`.

### 0.2 Design system
- Add `apps/web/src/lib/design/` with tokens (`colors.ts`, `space.ts`,
  `type.ts`) and primitives (`Button.svelte`, `Card.svelte`, `Modal.svelte`,
  `Toast.svelte`, `Skeleton.svelte`, `Chart.svelte`, `Tag.svelte`,
  `EmptyState.svelte`).
- Replace inline `<style>` blocks page-by-page during later tracks.
- Pick one font pair (e.g. Inter + Noto Sans JP / Noto Sans SC / Noto Sans KR
  loaded per language).
- Light + dark themes via CSS variables, defaulted to system preference, with
  override in Settings.

### 0.3 Configuration & errors
- `apps/web/src/lib/api.ts:5` — replace hardcoded `API_BASE` with
  `import.meta.env.VITE_API_BASE`. Add `.env.example`.
- `apps/extension/src/popup-page/app.ts:153` — replace
  `http://localhost:5173/cards` with a build-time const from
  `vite.config.ts`.
- Global error handling: every `apiFetch` failure routes through the toast
  region. Delete every silent `catch { /* no-op */ }` (`decks/+page.svelte`,
  `output/+page.svelte`, `library/+page.svelte`, `cards/+page.svelte`).

### 0.4 Acceptance
- Adding a new page is one file (`+page.svelte`) — the shell handles layout,
  auth, language, errors.
- `pnpm build` for the web app respects `VITE_API_BASE`.
- A failing API call shows a toast, not silence.

---

## Track 1 — Onboarding & landing (1–2 weeks)

The first 5 minutes are everything. Today they are a page of links.

### 1.1 Marketing routes (no auth)
- `routes/+page.svelte` → marketing landing (hero, value pillars,
  screenshots, "Install for Chrome / Firefox / Safari", "Sign up").
- `routes/pricing/+page.svelte` → public pricing page. Mirrors Stripe
  products from `services/api/internal/billing/`.
- `routes/docs/+page.svelte` (optional) or external docs site link.

### 1.2 Auth completion
- `routes/register/+page.svelte` (currently the login page has to also do
  this; split it).
- Password reset flow: `routes/forgot-password/`, `routes/reset-password/`,
  plus `auth.Handler` endpoints `POST /v1/auth/forgot`, `POST /v1/auth/reset`.
- Email verification on register (token + `POST /v1/auth/verify`).
- Account deletion endpoint (GDPR) in `services/api/internal/users/handler.go`
  plus Settings UI hook.

### 1.3 Onboarding wizard
- `routes/onboarding/+page.svelte`, shown once on first login.
  - **Step 1:** pick target language (JA / ZH / KO; ES/FR/DE/PT/IT greyed for
    Phase 5).
  - **Step 2:** known-words placement. Show ~50 frequency-bucketed words,
    user checks "I know these" → seeds `vocab_state` rows. (Server endpoint
    `POST /v1/onboarding/known-words`.)
  - **Step 3:** subscribe to starter deck (JLPT N5 if JA, HSK 1 if ZH,
    TOPIK 1 if KO).
  - **Step 4:** install extension link with browser auto-detect.
  - **Step 5:** "Mine your first card" tour — opens a sample content page in
    a new tab with an in-page overlay walking through hover → mine.
- Skipping is allowed but the home page shows the unfinished steps until
  done.

### 1.4 Acceptance
- A new user can register → confirm email → finish onboarding → see their
  first card in `/cards` without reading any docs.
- Pricing page is live and matches Stripe.

---

## Track 2 — Mining parity (3–5 weeks)

The headline feature. Without this, parity claim fails.

### 2.1 Audio + frame capture from video subtitles
- Refactor `apps/extension/src/content/video/` to:
  - Per-platform `getCurrentVideoElement()` (Netflix uses its `<video>` in
    the player shadow root; YouTube uses `.video-stream`).
  - On Mine: grab `video.currentTime` and the active subtitle cue's start/end
    from the SRT/WebVTT track if available, else fall back to a 4-second
    window around `currentTime`.
  - Capture a frame: draw `<video>` to an `OffscreenCanvas` →
    `convertToBlob({ type: 'image/jpeg', quality: 0.8 })`.
  - Capture audio: use `MediaRecorder` on
    `video.captureStream().getAudioTracks()[0]` for the cue window. Save as
    webm/opus.
  - Upload via new endpoint `POST /v1/cards/{id}/media` (multipart:
    `image`, `audio`) writing to `services/media` and returning signed URLs
    stored on the card.
- Card schema additions in a new migration `005_card_media.sql`:
  - `cards.image_url`, `cards.audio_url` already exist (`003`). Add
    `cards.video_source_url`, `cards.subtitle_start_ms`,
    `cards.subtitle_end_ms`, `cards.subtitle_translation`.

### 2.2 Subtitle UX overhaul
- New `apps/extension/src/content/video/SubtitleOverlay.ts`:
  - Renders a dual-line subtitle row docked above the native subtitle, with
    target subtitle (tokenized, click-to-popup) and optional native
    subtitle (toggle).
  - Hides the native Netflix/YouTube subtitle to avoid double-rendering.
  - Provides previous-cue / next-cue buttons (arrow keys ←/→).
  - Pause-on-subtitle toggle (auto-pause when a new cue begins).
  - Single-keystroke mine: pressing `M` while a cue is shown mines the
    current cue + its audio/frame; no popup required.
- Per-platform glue stays in `platforms/netflix.ts`, `youtube.ts`. Add
  `platforms/disneyplus.ts`, `platforms/amazonprime.ts`, `platforms/crunchyroll.ts`,
  `platforms/viki.ts` after netflix+youtube ship.

### 2.3 Popup-side mining additions
- `PopupManager.ts:125` — Mine button opens a small inline preview with
  editable fields: lemma, reading, definition (pre-filled, editable),
  sentence (pre-filled), translation (optional, pre-filled via lookup),
  notes, tags, deck selector. Save commits the card.
- Add `routes/cards/[id]/+page.svelte` (card detail view) so the Mine
  preview's "View card" link goes somewhere.
- For web-page mining (non-video), capture a screenshot of the surrounding
  paragraph: `html2canvas`-equivalent on the smallest text-containing
  ancestor.

### 2.4 Translation pair
- Server-side translation: new endpoint
  `POST /v1/nlp/translate` (proxied to a deterministic source: DeepL,
  Google, or Claude). Cached by `(source_lang, target_lang, sentence_hash)`.
- Popup and mining preview show the translation as a secondary line.

### 2.5 Acceptance
- On Netflix or YouTube with target-language subs: pressing `M` saves a card
  with sentence + frame screenshot + audio clip + translation, visible in
  `/cards` within 2 seconds.
- Reviewing that card plays the original audio and shows the original frame.

---

## Track 3 — Reader & import (2–3 weeks)

Reading anything that isn't a web page or a streamed video.

### 3.1 Reader mode for library URLs
- `routes/library/[id]/+page.svelte`:
  - Server fetches the URL through `services/media` (already exists),
    extracts main text via Mozilla Readability, runs the NLP tokenizer.
  - Renders the tokenized text with the same annotator styles as the
    extension. Click-to-popup, mining, audio playback, all in-app.
  - Sidebar lists the unknown words for the article, sorted by frequency,
    with a "Pre-learn these 12 words" button that adds them to today's new
    queue.

### 3.2 Plain-text + EPUB + SRT import
- `routes/import/+page.svelte` plus
  `services/api/internal/library/handler.go`:
  - Accept `.txt`, `.srt`, `.epub` upload. Store in `media`. Reader mode
    works on the imported item just like a URL.
  - For `.srt`, render in subtitle-stepper mode (one cue at a time, audio
    optional via parallel upload).

### 3.3 Anki + Migaku + Yomitan import
- `routes/import/+page.svelte` second tab:
  - `.apkg` import — parses the Anki package, maps fields to Carve cards
    (configurable: which Anki field is `front`, `back`, `sentence`, `audio`,
    `image`). Creates a new deck named after the apkg.
  - Migaku CSV import.
  - Yomitan vocab JSON import (creates `vocab_state` rows with `known`
    status, no cards).
  - JPDB known-words CSV import.
- Server work in `services/api/internal/import/` (new package). Parses
  apkg via `archive/zip` + SQLite reader. Stores media to S3, rewrites paths.

### 3.4 AnkiConnect bridge (optional coexist mode)
- Settings panel: "Sync cards to Anki" with AnkiConnect URL field.
- Background worker in API pushes new cards / review updates to AnkiConnect.
- Two-way: pulls Anki review events back on schedule.

### 3.5 Acceptance
- A user with a 10k-card Anki deck can import in <5 minutes and start
  reviewing in Carve immediately.
- A library URL renders as annotated reader text with mining parity to the
  extension.

---

## Track 4 — Power-user review (1–2 weeks)

Daily-loop quality. Without this users drift back to Anki.

### 4.1 Keyboard shortcuts
- `routes/review/+page.svelte`: `Space` flip; `1/2/3/4` rate; `Z` undo; `E`
  edit; `S` suspend; `B` bury; `A` play audio; `M` mark leech; `?` shortcut
  help overlay.

### 4.2 Card editing & lifecycle
- Add `PATCH /v1/cards/{id}` in `services/api/internal/cards/handler.go`.
- Card detail page (from 2.3) gets edit mode: lemma, reading, definition,
  sentence, translation, notes, tags, deck.
- `POST /v1/cards/{id}/suspend`, `…/bury`, `…/unsuspend`.
- `POST /v1/review/undo` reverts the last review event (idempotent within
  10 minutes).

### 4.3 Cards page upgrade
- Search box (full-text on lemma + reading + sentence).
- Filter: state, deck, language, has-audio, has-image, suspended, leech.
- Sort: due, created, last-reviewed, lapses, alphabetical.
- Bulk select → tag, move-to-deck, suspend, delete.

### 4.4 Real pitch + furigana modes
- Replace pitch label rendering (`PopupManager.ts:217`) with an inline SVG
  contour drawing the pitch curve over the mora.
- Furigana mode setting: `always | unknown-only | off | kanji-only`. Wired
  in extension `injectStyles()` and in reader mode.
- Pitch-color setting: tone color overlay on/off; color-blind palette.

### 4.5 Acceptance
- 100 reviews in a row, mouse never touched.
- Editing a wrong reading takes <10 seconds, including persisting.

---

## Track 5 — Cross-browser (2–3 weeks)

Match the README before launch.

### 5.1 Manifest matrix
- Split into `manifest.chrome.json`, `manifest.firefox.json`,
  `manifest.safari.json`. Build script picks per `BROWSER=chrome|firefox|safari`.
- Firefox-specific: `browser_specific_settings.gecko.id`, MV3-on-Firefox
  service-worker → background scripts fallback for older versions.
- Safari: generate Xcode wrapper project via `xcrun safari-web-extension-converter`.
  Add `apps/extension/safari/` with the project, committed.
- Update `apps/extension/package.json` `build` script to
  `BROWSER=chrome pnpm build:browser && BROWSER=firefox … && BROWSER=safari …`.

### 5.2 API surface parity
- Replace any `chrome.*` direct calls with `browser.*` via
  `webextension-polyfill`. Audit needed in:
  `background/index.ts`, `popup-page/app.ts`, `content/index.ts`,
  `content/popup/PopupManager.ts`.

### 5.3 Submission readiness
- Privacy policy page on web app (`routes/privacy/`).
- Store listing copy + screenshots for Chrome Web Store, Firefox AMO,
  Apple App Store Connect.
- Reproducible build (CI artifact per browser).

### 5.4 Acceptance
- One PR pipeline produces three signed extension packages.
- Each installs and runs the full Track 2 mining flow on its browser.

---

## Track 6 — PWA & mobile review (2 weeks)

Phase 3 deliverable, currently zero. Native mobile remains separate (Phase 5).

### 6.1 PWA basics
- `apps/web/static/manifest.webmanifest` with icons, theme color, display:
  standalone.
- `apps/web/src/service-worker.ts` (SvelteKit native): pre-cache shell,
  runtime-cache `/v1/review/session` for offline replay.
- `apps/web/static/icons/` set (192, 256, 384, 512).
- Install prompt component, shown once on supported browsers after first
  successful review.

### 6.2 Offline review
- Background sync: queued review events stored in IndexedDB, replayed on
  reconnect. Extension already has this in `shared/storage.ts`; mirror to
  the web app.

### 6.3 Touch-optimized review
- Variant of `routes/review/+page.svelte` for `(max-width: 720px)`:
  - Full-screen card.
  - Swipe left → Again, down → Hard, up → Good, right → Easy. Configurable.
  - Long-press card to flip; tap audio button to replay.
  - Bottom-bar fallback buttons for non-swipe users.

### 6.4 Acceptance
- Installable on iOS Safari and Android Chrome as a home-screen app.
- 50 reviews complete in airplane mode; events sync on reconnect.

---

## Track 7 — Output completion (2 weeks)

### 7.1 Mic capture + STT for shadowing & speaking
- Add `routes/output/shadowing/[id]/+page.svelte` and
  `routes/output/speaking/[id]/+page.svelte`:
  - `MediaRecorder` on the user's mic.
  - Post the audio to `POST /v1/output/transcribe` (Whisper API or
    self-hosted faster-whisper) → text.
  - Word-level diff against the expected transcript; mismatches highlighted.
  - Pitch / pause overlay against the reference audio (FFT both, draw
    waveforms).

### 7.2 Daily output session
- `routes/output/+page.svelte` becomes a session, not a list: today's queue
  is `min(N, recently_mined ∪ leeches ∪ relearning)` matching the cards
  user has actually seen in review this week.

### 7.3 Acceptance
- After a week of mining, a user opens `/output` and gets ~10 focused
  exercises spanning writing, cloze, shadowing, and speaking, all built from
  their own recent cards.

---

## Track 8 — Stats, notifications, settings (1 week)

### 8.1 Stats
- Replace `{@html chartBars(...)}` with `Chart.svelte` (SVG, axes, hover
  tooltips).
- Add per-language tabs, per-deck retention table, calendar heatmap of
  reviews/day, true-vs-first-attempt retention toggle.

### 8.2 Notifications
- Bell in top bar (Track 0) shows unread count; dropdown lists
  notifications from `/v1/review/notifications`.
- Browser push (web-push API) for "X cards due in the morning" opt-in.

### 8.3 Settings expansion
- `routes/settings/+page.svelte` becomes tabbed:
  - **Account** — email, password, delete account, data download.
  - **Display** — theme, font size, furigana mode, pitch mode, palette,
    UI language.
  - **Review** — FSRS (existing), daily new limit per language, suspend
    threshold.
  - **Mining** — default deck per language, screenshot quality, auto-pause,
    keybinds.
  - **Sites** — list of disabled domains with toggles.
  - **Sync** — AnkiConnect URL, last sync time, sync button.
  - **Billing** — current plan, upgrade/downgrade link to Stripe portal.

### 8.4 Acceptance
- Every UX preference a user might want is in one tabbed settings page.

---

## Track 9 — Tokenizer accuracy & reliability (ongoing, but gated)

The phase-0 quality gate stays in force. Reliability work that should land
before public beta:

- **WASM ZH tokenizer** (`apps/extension/wasm-src/zh-tokenizer/`): publish
  `pkg/` via `wasm-pack`. Bring ZH correctness suite to 100% pass.
- **WASM KO tokenizer**: same.
- **Fallback hierarchy**: WASM (fast, in-process) → API
  (`/v1/nlp/tokenize`) → no-op. Currently `PageAnnotator.tokenize()` already
  does this for non-JA; KO/ZH should mirror.
- **Cold-start fix for SudachiPy** — server first-call latency is
  ~120s (per `cmd/api/main.go:130`). Pre-warm in `services/nlp` startup,
  cache the loaded dictionary in shared memory.
- **Pitch accent data** — confirm OJAD/NHK source license and complete
  coverage; currently only some entries return `pitch_accent`.

---

## Track 10 — Reliability & operations (1–2 weeks, parallel)

### 10.1 Observability
- Structured logs already exist (`slog`). Add request-ID propagation through
  every handler (currently only chi middleware emits it).
- Add Prometheus metrics endpoint to API + NLP service. Track per-endpoint
  latency, error rate, FSRS event throughput.
- Sentry (or self-hosted GlitchTip) for the web app and extension.

### 10.2 Testing
- Add web e2e suite under `/e2e` (Playwright). Smoke: register → onboarding
  → mine a card on a fixture page → review → see stats update.
- Extension e2e: load unpacked into headless Chromium, run a scripted
  Netflix-mock page, verify mining round-trip.
- Backend integration tests for: import (apkg), translate, media upload,
  AnkiConnect bridge.

### 10.3 CI & release
- GitHub Actions matrix: lint + test + build for web, api, nlp, extension
  (×3 browsers). Required on every PR.
- Tagged releases produce: signed extension packages, web app docker image,
  api docker image, nlp docker image. Publish to GHCR.
- Database migrations: pin to `goose` (`services/api/migrations/`); add
  `make migrate-up` / `make migrate-down`; CI runs against a fresh Postgres
  per build.

### 10.4 Self-host docs
- `docs/12-self-hosting.md`: one-command `docker compose up` story end to
  end. Dictionary downloads scripted (`scripts/seed-dicts.sh`).

---

## Sequencing and rough timeline

Assume one engineer full-time, two part-time. Tracks can parallelise where
they don't share files.

| Weeks | Tracks | Outcome |
|---|---|---|
| 1–2 | 0 | Foundation, design system, app shell. |
| 3–4 | 1 | Landing, onboarding, auth complete. |
| 3–7 | 2 (parallel with 1) | Mining parity with Migaku. |
| 6–8 | 4 (after 2 lands cards/edit) | Keyboard review + edit + bulk. |
| 7–9 | 3 (parallel) | Reader + import (Anki/Migaku). |
| 9–11 | 5 | Firefox + Safari shipped. |
| 11–12 | 6 | PWA + touch review. |
| 12–13 | 7 | Output completion. |
| 13–14 | 8 | Stats + notifications + settings. |
| ongoing | 9, 10 | Tokenizer reliability + ops. |

**Public beta gate:** Tracks 0, 1, 2, 4, 5 complete; Track 9 at 100% JA / 95%
ZH / 90% KO test pass; one P1 bug open, no P0.

---

## Definition of "drop-in Migaku replacement"

A user who currently uses Migaku must be able to:

1. Install a Carve extension on **their** browser (Chrome / Firefox / Safari).
2. Import their existing Anki / Migaku deck without losing review history.
3. Open the same Netflix episode they were watching and mine cards with the
   same gestures, getting the same artifacts (sentence + audio + frame +
   translation).
4. Review on web *and* mobile, with the same keyboard / swipe shortcuts they
   already know.
5. See their known-words count, comprehension scores, and immersion time
   carry over from imported data.
6. Cancel Migaku without missing any feature they actually used.

Each track above is a step toward that bar. Track 2 + Track 3 + Track 5
together are the minimum subset that earns the "drop-in" claim.
