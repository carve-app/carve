# State-of-Project Audit (2026-05-30)

> **Point-in-time snapshot — superseded.** For the current verified state see
> [STATUS.md](STATUS.md). Kept as a historical audit record.

> Snapshot of the codebase against the "better Migaku" promise: feature
> completeness, UX, design, polished workflows. Captured after Phase 4 work
> landed (Chinese/Korean tokenizer scaffolds, output practice, billing).

---

## What is actually built

| Area | State | Notes |
|---|---|---|
| NLP core (JA) | Strong | SudachiPy server tokenizer + WASM build, 1,900-line correctness suite, 14-file corpus. |
| NLP (ZH/KO) | Scaffold | Python tokenizers exist; WASM `pkg/` not yet published for ZH; tests exist but coverage thin. |
| Extension (Chrome) | Functional | Page annotator, click-popup, mining, Netflix + YouTube subtitle hooks, immersion tracker. |
| API | Broad | Auth, cards, review, FSRS-6 + optimizer, intervals, forecast, leech, decks (CRUD+rate+sub), library, stats, output (writing/cloze/shadow), Stripe, export, notifications. |
| Web app | Routes present | review, cards, decks, library, stats, settings, output, export, login. |
| Phase 5+ | Not started | No European languages, no native mobile, no content recommendations. |

So the **checkbox state** through Phase 4 is largely green. The **product state**
is alpha; the gap is in design, workflow polish, and the few features that
actually define Migaku.

---

## Gap 1 — The product has no front door

`apps/web/src/routes/+page.svelte` is a literal `<nav>` of 9 links on a dark
background.

- No landing page, no value prop, no screenshots, no pricing page — despite
  README pitching "transparent pricing."
- No onboarding. After register/login the user lands on the same nav list. No
  language selector, no JLPT placement, no "subscribe to the N5 starter deck"
  CTA, no extension-install prompt.
- No app shell / shared layout. Every route hand-rolls its own `<header><nav>`
  and dark theme — no `+layout.svelte`. Adding a page means re-pasting the
  header.
- No global notification UI, even though `/v1/review/notifications` is wired
  in `apps/web/src/lib/api.ts:217`.

## Gap 2 — Cross-browser is a README claim, not a build

- `apps/extension/src/manifest.json` is single-file Chrome MV3.
- No `manifest_firefox.json`, no `browser_specific_settings`, no
  Safari Web Extension wrapper, no per-browser build target in
  `apps/extension/package.json` (`build` script only emits Chrome).
- README and `docs/02-product-design.md` headline cross-browser as a top
  differentiator. Today it is false advertising.

## Gap 3 — Mobile / PWA is missing entirely

- No `webmanifest`, no service worker registration in `apps/web/src/app.html`,
  no `static/icons/`, no offline shell. `static/` is empty.
- Roadmap Phase 3 promises "Mobile PWA: review sessions work offline on
  mobile." Nothing in `apps/web` implements this.
- Review UI (`routes/review/+page.svelte:182`) uses small click targets and a
  `Show answer` button — no swipe-to-rate, no large thumb-zone buttons, no
  haptics. Migaku Memory's mobile UX is one of its real strengths; we have
  none of it.

## Gap 4 — Mining workflow is anemic vs. Migaku

This is the headline workflow and it is the weakest area in code.

- `content/popup/PopupManager.ts:125` — the Mine button sends only
  `{lemma, reading, sentence, sourceUrl}`. No image capture, no audio capture,
  no translation field, no user notes, no deck selection.
- `content/video/platforms/netflix.ts` and `youtube.ts` tokenize the current
  subtitle line only. **Migaku's flagship**: click a subtitle → card with the
  full sentence + an audio clip of that subtitle + a video frame screenshot +
  a translation pair. None of that exists. No `<video>` `currentTime` capture,
  no canvas frame grab, no audio extraction, no SRT scrubber, no
  previous/next subtitle stepper.
- No dual-subtitle display (target + native).
- No keyboard shortcut to mine the currently-displayed subtitle.
- Migration `003_audio_reading.sql` adds `front_audio_url` columns, but the
  extension never sets them — audio is lazy-fetched server-side from
  Forvo/JPod101 (per roadmap). That is the inferior path; learners want the
  *actual* line they heard.

## Gap 5 — Popup is missing the bits learners actually use

`PopupManager.ts` shows lemma, reading, JLPT, freq, pitch label, status,
sentence, top-3 definitions, Mine, Ignore. Missing vs. Yomitan/Migaku:

- No example sentences from Tatoeba, even though the migration imports them.
- No audio playback button on the popup itself.
- No conjugation tree (`食べられた = 食べる + passive + past`).
- No related forms / synonyms / antonyms.
- No "this is part of compound X" hint.
- Pitch accent renders as `[1①]` / `[0⓪]` (`PopupManager.ts:217`) — not the
  standard line-over-mora curve. Pitch learners will reject this.
- Furigana ruby works but with no toggle (always-on / unknown-only / off) and
  no kanji-only mode.

## Gap 6 — Card management UI is read-only

`routes/cards/+page.svelte`:
- No edit. Can't fix a wrong reading, change definition, add notes/tags.
- No bulk actions (suspend, tag, delete, move to deck).
- No filter / search / sort. Becomes unusable past ~1,000 cards.
- No per-card detail view, no review-history graph.
- API only exposes Create/List/Delete on cards
  (`services/api/internal/cards/handler.go`) — no `PATCH`.

## Gap 7 — Decks: discovery is broken

`routes/decks/+page.svelte`:
- No deck detail page. Can't preview cards before subscribing.
- No tag filter, no language filter, no search box, no sort
  (popular/new/best-rated).
- No featured / official row.
- No `.apkg` import. No CSV import. The Migaku/Anki ecosystem the product
  intends to replace has no migration path.

## Gap 8 — Language is silently hardcoded to `ja`

Every web page hardcodes `'ja'`:

- `routes/review/+page.svelte:67` `fetchReviewSession('ja', 20)`
- `routes/cards/+page.svelte:46` `fetchCards('ja', 50)`
- Same in `stats`, `library`, `settings`, `output`.
- Extension popup `popup-page/app.ts:198` `/v1/review/due-count?language=ja`.
- `content/index.ts:52` `detectLanguage()` returns `'ja'` or `null` only — it
  will false-positively flag Chinese pages as Japanese (CJK regex match) and
  will never engage on Korean.

There is no global language switcher anywhere. A user with three target
languages can use exactly one.

## Gap 9 — Library is a URL list, not a reader

`routes/library/+page.svelte` is a list of URLs with a comprehension %.

- No reader mode (distraction-free annotated text view).
- No EPUB / SRT / plain-text import (Phase 5 has EPUB; nothing earlier).
- No content recommendations engine ("you know 3,200 words → here is NHK Easy
  at 95%").
- No vocab pre-load per item ("study these 12 unknown words before reading").

## Gap 10 — Output practice is the right shape but shallow

`routes/output/+page.svelte` + `services/api/internal/output/handler.go`:

- Writing & cloze go through Claude → 0–100 score + 3-axis feedback. OK.
- Shadowing tab is "play audio, click 'mark done'." No mic recording, no
  waveform compare, no STT diff. Roadmap promised
  "user records → STT → diff against expected" — not implemented.
- No "today's output session" generated from recent mining. Currently a flat
  list of all exercises.

## Gap 11 — Stats are barely interpretive

`routes/stats/+page.svelte`: four cards, a known-words bar chart, a 14-day
forecast, three text rows.

- No per-language breakdown despite multi-language being the pitch.
- No retention split by deck or by card age.
- No "true retention" vs. "first-attempt retention" toggle.
- No heatmap calendar.
- Charts are inline HTML strings (`{@html chartBars(...)}`) — no axes, no real
  tooltips. Replace with hand-rolled SVG or a chart lib.

## Gap 12 — Settings page covers ~20% of expected scope

`routes/settings/+page.svelte` is FSRS-only. Missing:

- Per-language settings (separate retention, separate daily-new, separate
  default deck).
- Display: theme, font size, furigana mode, pitch mode, color-blind palette.
- Extension: hotkeys, default deck for mining, screenshot quality, list of
  disabled domains (currently only togglable inside the extension popup with
  no global view).
- Audio source preference (Forvo / JPod101 / TTS).
- Account: change password, change email, delete account (GDPR), separate
  data download.
- Notifications & email preferences.
- Anki bridge: AnkiConnect URL, deck mapping, auto-sync toggle.

## Gap 13 — No keyboard-first review

`routes/review/+page.svelte` has no keyboard handlers — no Space to flip, no
1/2/3/4 to rate, no Z to undo, no E to edit, no S to suspend, no A to play
audio. Mouse only. Power users review hundreds of cards/day.

## Gap 14 — Polish details that signal "rough alpha"

- `apps/web/src/lib/api.ts:5` hardcodes `API_BASE = 'http://localhost:8080'`.
  No env-based config — will break on first staging build.
- Extension popup `popup-page/app.ts:153` opens `http://localhost:5173/cards`
  — same hardcoded-URL issue.
- Many client catches are silent `catch { /* no-op */ }`
  (`routes/decks/+page.svelte:74`, `routes/output/+page.svelte:91`,
  `library/+page.svelte:59`). Failures invisible to the user.
- Every page reimplements its own auth check
  (`if (!localStorage.getItem('carve_access_token'))`) instead of a guard hook
  or `+layout.server.ts`.
- No loading skeletons; every page is `<p>Loading…</p>`.
- No focus traps in popups/dialogs, no `role="dialog"`, no `Escape` handlers
  beyond the extension popup.
- No i18n framework — UI is English-only. For a language-learning product,
  ironic.
- No favicons, no og-tags, no SEO setup on `app.html`.

## Gap 15 — No migration path from Migaku / Anki / Yomitan

The target user has hundreds of existing cards somewhere. There is:

- No `.apkg` import.
- No Migaku CSV import.
- No Yomitan-vocab list import.
- No JPDB known-words import.
- No AnkiConnect bridge for users who want to coexist.

Without these, switching means abandoning years of decks — a non-starter.

## Gap 16 — Anki-style essentials still missing

- No "undo last review" button.
- No "bury card / bury siblings".
- No manual "suspend card" action (only auto-suspend on leech).
- No card preview before mining — you click Mine, the popup closes, you can't
  see what got saved.
- No notes / tags on cards (no schema field surfaced).

---

## Severity ranking for the "better Migaku" claim

| # | Gap | Why it matters |
|---|---|---|
| 1 | Subtitle mining with audio + frame capture | Defines Migaku. Without it Carve is "Yomitan + SRS." |
| 2 | Onboarding + landing + app shell | Cannot survive public beta. |
| 3 | Anki / Migaku import | Removes the only blocker to switching. |
| 4 | Language switcher + de-hardcoded `'ja'` | Multi-language is the second-loudest claim. |
| 5 | Firefox + Safari builds | Cross-browser is the first-loudest claim. |
| 6 | Card edit + bulk + search | Required past ~500 cards. |
| 7 | Keyboard shortcuts in review | Power-user table stakes. |
| 8 | Real pitch viz + furigana modes | JA learner table stakes. |
| 9 | PWA + touch-optimized review | Phase 3 deliverable, currently zero. |
| 10 | Speaking drill + mic capture | Completes the "output" pillar. |

The backend plumbing is mostly there. The frontend, the mining workflow, and
the onboarding are where the product currently fails the bar.
