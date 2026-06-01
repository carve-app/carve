# User Flow Inventory

> Every user-visible flow Carve supports, mapped to the E2E spec that
> covers it. Anything in the "uncovered" column blocks the
> "drop-in Migaku replacement + all flows fully tested" gate.

## Web — public

| Flow                                       | E2E spec                                   |
|--------------------------------------------|--------------------------------------------|
| Landing render                             | `tests/journey-register-onboard.spec.ts`   |
| Pricing page                               | `tests/visual.spec.ts`, `tests/journey-pricing-billing.spec.ts` |
| Privacy page                               | `tests/visual.spec.ts`                     |
| Register (email + password)                | `tests/journey-register-onboard.spec.ts`   |
| Login                                      | `tests/journey-login.spec.ts`              |
| Forgot password → email link               | `tests/journey-password-reset.spec.ts`     |
| Reset password (token in URL)              | `tests/journey-password-reset.spec.ts`     |
| Email verification                         | `tests/journey-email-verify.spec.ts`       |
| Logout                                     | `tests/journey-login.spec.ts`              |

## Web — app shell

| Flow                                       | E2E spec                                   |
|--------------------------------------------|--------------------------------------------|
| Auth guard redirects to /login             | `tests/journey-login.spec.ts`              |
| Language switcher in top bar               | `tests/journey-language-switch.spec.ts`    |
| Sidebar navigation between routes          | `tests/journey-shell-nav.spec.ts`          |
| Toast surface (error + success)            | `tests/journey-toast.spec.ts`              |
| Theme respects system preference           | `tests/journey-theme.spec.ts`              |

## Onboarding wizard

| Flow                                       | E2E spec                                   |
|--------------------------------------------|--------------------------------------------|
| Step 1 — pick language (incl. EN)          | `tests/journey-register-onboard.spec.ts`   |
| Step 2 — known-words placement             | `tests/journey-register-onboard.spec.ts`   |
| Step 3 — subscribe starter deck            | `tests/journey-register-onboard.spec.ts`   |
| Step 4 — extension install link            | `tests/journey-register-onboard.spec.ts`   |
| Step 5 — mining tour                       | `tests/journey-register-onboard.spec.ts`   |
| Skip onboarding                            | `tests/journey-register-onboard.spec.ts`   |

## Cards

| Flow                                       | E2E spec                                   |
|--------------------------------------------|--------------------------------------------|
| List cards with filters                    | `tests/journey-cards-list.spec.ts`         |
| Bulk select → tag / move / suspend / delete| `tests/journey-cards-bulk.spec.ts`         |
| Edit card detail                           | `tests/journey-cards-edit.spec.ts`         |
| Suspend / unsuspend                        | `tests/journey-cards-edit.spec.ts`         |
| Bury card                                  | `tests/journey-review-shortcuts.spec.ts`   |
| Card detail audio playback                 | `tests/journey-cards-edit.spec.ts`         |

## Review

| Flow                                       | E2E spec                                   |
|--------------------------------------------|--------------------------------------------|
| Keyboard shortcuts (Space/1-4/Z/E/S/B/A/?) | `tests/journey-review-shortcuts.spec.ts`   |
| Touch swipe gestures                       | `tests/journey-review-swipe.spec.ts`       |
| Offline review queues to IndexedDB         | `tests/pwa-offline.spec.ts`                |
| Online reconnect flushes queue             | `tests/pwa-offline.spec.ts`                |
| Undo                                       | `tests/journey-review-shortcuts.spec.ts`   |
| Session-complete summary                   | `tests/journey-mine-review.spec.ts`        |
| Leech detection                            | `tests/journey-review-leech.spec.ts`       |

## Library / import

| Flow                                       | E2E spec                                   |
|--------------------------------------------|--------------------------------------------|
| URL import → reader mode                   | `tests/journey-library-reader.spec.ts`     |
| .txt import                                | `tests/journey-import-txt.spec.ts`         |
| .srt import (subtitle stepper)             | `tests/journey-import-srt.spec.ts`         |
| .epub import                               | `tests/journey-import-epub.spec.ts`        |
| Anki .apkg import with scheduling preserved| `internal/importer/anki_integration_test.go` (L3) |
| Migaku CSV import                          | `tests/journey-import-migaku.spec.ts`      |
| Yomitan vocab JSON import                  | `tests/journey-import-yomitan.spec.ts`     |
| JPDB known-words CSV                       | `tests/journey-import-jpdb.spec.ts`        |
| AnkiConnect sync (push + pull)             | `tests/journey-ankiconnect.spec.ts`        |

## Output

| Flow                                       | E2E spec                                   |
|--------------------------------------------|--------------------------------------------|
| Writing exercise → submit                  | `tests/journey-output-writing.spec.ts`     |
| Cloze exercise                             | `tests/journey-output-cloze.spec.ts`       |
| Shadowing record + transcribe + diff       | `tests/journey-output-shadowing.spec.ts`   |
| Speaking prompt + use target word          | `tests/journey-output-speaking.spec.ts`    |

## Stats

| Flow                                       | E2E spec                                   |
|--------------------------------------------|--------------------------------------------|
| Reviews-per-day chart                      | `tests/journey-stats.spec.ts`              |
| Calendar heatmap                           | `tests/journey-stats.spec.ts`              |
| Per-language tabs                          | `tests/journey-stats.spec.ts`              |
| Retention table                            | `tests/journey-stats.spec.ts`              |

## Settings (tabbed)

| Flow                                       | E2E spec                                   |
|--------------------------------------------|--------------------------------------------|
| Account tab — email/password change        | `tests/journey-settings-account.spec.ts`   |
| Account tab — delete account               | `tests/journey-settings-account.spec.ts`   |
| Account tab — data download                | `tests/journey-settings-account.spec.ts`   |
| Display tab — theme/font/furigana/pitch    | `tests/journey-settings-display.spec.ts`   |
| Review tab — FSRS weights, daily new limit | `tests/journey-settings-review.spec.ts`    |
| Mining tab — default deck, keybinds        | `tests/journey-settings-mining.spec.ts`    |
| Sites tab — disabled-domains list          | `tests/journey-settings-sites.spec.ts`     |
| Sync tab — AnkiConnect URL                 | `tests/journey-ankiconnect.spec.ts`        |
| Billing tab — Stripe portal redirect       | `tests/journey-pricing-billing.spec.ts`    |

## Extension (jsdom + Chromium fixture)

| Flow                                       | E2E spec                                   |
|--------------------------------------------|--------------------------------------------|
| Page tokenization (JA/ZH/KO/EN)            | `e2e/extension.test.js`                    |
| Click word → popup                         | `e2e/extension.test.js`                    |
| Mine card form save                        | `e2e/extension.test.js`                    |
| Subtitle hook fires per platform           | `__tests__/platforms.test.ts` (×6 platforms)|
| Subtitle M-key mines current cue           | `tests/extension-subtitle-mine.spec.ts`    |
| Comprehension overlay toggle               | `tests/extension-popup.spec.ts`            |
| Per-site disable toggle                    | `tests/extension-popup.spec.ts`            |
| Target-language selector                   | `tests/extension-popup.spec.ts`            |
| Login from popup                           | `tests/extension-popup.spec.ts`            |

## PWA

| Flow                                       | E2E spec                                   |
|--------------------------------------------|--------------------------------------------|
| Install prompt after first review          | `tests/pwa-install.spec.ts`                |
| Service worker registers + caches          | `tests/pwa-sw.spec.ts`                     |
| Manifest declares icons + theme            | `tests/pwa-manifest.spec.ts`               |
| Offline review (50-card airplane mode)     | `tests/pwa-offline.spec.ts`                |

## Production health

| Flow                                       | Coverage                                   |
|--------------------------------------------|--------------------------------------------|
| Synthetic register→mine→review per minute  | `scripts/synthetic.mjs` (L15)              |
| Streaming-platform DOM canary hourly       | `tests/extension-streaming.spec.ts` (L11)  |
| Prometheus /metrics scrape                 | `internal/metrics/metrics_test.go`         |

---

A flow without a row in this table is, by definition, not part of the
shipped product. To add one, write the spec first, then the feature.
