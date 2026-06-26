# User Flow Inventory

This inventory describes enforced evidence in the repository. `Covered` means
the cited test executes the behavior, `Partial` names the missing proof, and
`Unverified` is never treated as a passing result. CI runs
`scripts/verify-flow-inventory.mjs` so references cannot silently point at
nonexistent specs.

## Public web and authentication

| Flow | Status | Enforced evidence |
|---|---|---|
| Landing, registration, onboarding | Covered with mock API | `e2e/tests/journey-register-onboard.spec.ts` |
| Login, auth guard, logout | Covered with mock API | `e2e/tests/journey-login.spec.ts` |
| Forgot/reset password UI | Covered with mock API | `e2e/tests/journey-password-reset.spec.ts` |
| Email verification error/resend UI | Covered with mock API | `e2e/tests/journey-email-verify.spec.ts` |
| Real SMTP verification and reset | Covered in isolated stack | `e2e/tests/real-stack-core.spec.ts` |
| Pricing and privacy | Covered | `e2e/tests/journey-pricing.spec.ts` |

## Application shell and learning flows

| Flow | Status | Enforced evidence |
|---|---|---|
| Primary navigation under one token | Covered with mock API | `e2e/tests/journey-shell-nav.spec.ts` |
| Language selection persists | Covered with mock API | `e2e/tests/journey-language-switch.spec.ts` |
| Cards list and detail form accessibility | Covered with mock API | `e2e/tests/journey-cards.spec.ts` |
| Card edits, bulk actions, suspend/bury | Partial: handler coverage exists; full UI mutation journey is still backlog | `services/api/internal/cards/handler_test.go` |
| Keyboard review journey | Covered with mock API | `e2e/tests/journey-mine-review.spec.ts` |
| Review scheduling and undo | Covered at service level | `services/api/internal/review/handler_test.go` |
| Retry-safe review event | Covered against real Postgres | `services/api/internal/review/idempotency_integration_test.go` |
| Fifty offline reviews and exact queue drain | Covered with deterministic browser/API state | `e2e/tests/pwa-offline.spec.ts` |
| Stats route and visual primitives | Covered with mock API | `e2e/tests/journey-stats.spec.ts` |
| Settings tab accessibility | Covered with mock API | `e2e/tests/journey-settings-tabs.spec.ts` |
| Output landing, shadowing, speaking empty states | Partial: rendering only | `e2e/tests/journey-output.spec.ts` |

## Import, export, and library

| Flow | Status | Enforced evidence |
|---|---|---|
| Import UI tabs and format selection | Covered with mock API | `e2e/tests/journey-import.spec.ts` |
| Anki scheduling preservation | Covered against real Postgres | `services/api/internal/importer/anki_integration_test.go` |
| Anki/Migaku/Yomitan/JPDB parser behavior | Covered at service level | `services/api/internal/importer/handler_test.go` |
| Anki and CSV export structure | Covered at service level | `services/api/internal/export/export_test.go` |
| URL reader and TXT/SRT import | Covered at service level | `services/api/internal/library/handler_test.go` |
| EPUB import | Unverified: not shipped; API explicitly accepts only TXT/SRT |
| AnkiConnect push/pull | Partial: transport behavior covered, no real Anki process in CI | `services/api/internal/sync/ankiconnect_test.go` |

## Extension, video, and PWA

| Flow | Status | Enforced evidence |
|---|---|---|
| Page annotation, popup lookup, mining save | Covered in packaged Chromium with mock API | `e2e/extension.test.js` |
| Recorded streaming DOM hooks | Covered for six platforms | `apps/extension/src/content/video/__tests__/platforms.test.ts` |
| Real-stack video media mining | Covered in Chromium | `e2e/video-mining.test.js` |
| Public live YouTube subtitle cue | Failed on 2026-06-26: overlay mounted, but anonymous YouTube exposed no caption cue; hourly canary remains blocking | `e2e/tests/live-youtube-canary.spec.ts` |
| PWA manifest and service-worker registration | Covered | `e2e/tests/pwa-manifest.spec.ts` |
| Chrome package/runtime | Covered |
| Firefox package/runtime | Covered with real Firefox + `web-ext` | `e2e/firefox-extension-smoke.cjs` |
| Safari package | Unverified: bundle builds, but no native Safari Web Extension wrapper/signing project exists |

## Provider and production status

| Flow | Status | Evidence or limitation |
|---|---|---|
| Nine dictionaries/tokenizers plus Vietnamese tokenization | Covered locally | `services/nlp/tests/test_correctness.py` |
| Google Translation/TTS | Inconclusive when service-account credentials are unavailable |
| Anthropic contextual explanation | Inconclusive when API credentials are unavailable |
| Authenticated Netflix/Disney+/Prime/Crunchyroll/Viki live playback | Inconclusive without active provider sessions; recorded fixtures remain covered |
| Production register/create/review synthetic | Available but requires explicit production configuration | `scripts/synthetic.mjs` |

Adding a shipped flow requires an enforced test or an explicit `Partial` or
`Unverified` entry. Aspirational test filenames are not accepted as evidence.
