# User Flow Inventory

This inventory describes enforced evidence in the repository. `Covered` means
the cited test executes the behavior, `Partial` names the missing proof, and
`Unverified` is never treated as a passing result. CI runs
`scripts/verify-flow-inventory.mjs` so references cannot silently point at
nonexistent specs.

## Public web and authentication

| Flow | Status | Enforced evidence |
|---|---|---|
| Landing, registration, onboarding | Covered with mock UI and real API persistence | `e2e/tests/journey-register-onboard.spec.ts`, `e2e/tests/real-stack-core.spec.ts` |
| Login, auth guard, refresh rotation, logout | Covered with mock UI and isolated real stack | `e2e/tests/journey-login.spec.ts`, `e2e/tests/real-stack-core.spec.ts` |
| Forgot/reset password UI | Covered with mock API | `e2e/tests/journey-password-reset.spec.ts` |
| Email verification error/resend UI | Covered with mock API | `e2e/tests/journey-email-verify.spec.ts` |
| Real SMTP verification and reset | Covered in isolated stack through Mailpit | `e2e/tests/real-stack-core.spec.ts` |
| Pricing and privacy | Covered | `e2e/tests/journey-pricing.spec.ts` |

## Application shell and learning flows

| Flow | Status | Enforced evidence |
|---|---|---|
| Primary navigation under one token | Covered with mock API | `e2e/tests/journey-shell-nav.spec.ts` |
| Language selection persists | Covered with mock API | `e2e/tests/journey-language-switch.spec.ts` |
| Cards list and detail form accessibility | Covered with mock API | `e2e/tests/journey-cards.spec.ts` |
| Card edits, duplicate detection, media, bulk, suspend/bury/unbury | Covered against the real API; full UI mutation journey remains backlog | `e2e/tests/real-stack-core.spec.ts` |
| Keyboard review journey | Covered with mock API | `e2e/tests/journey-mine-review.spec.ts` |
| Daily limits, scheduling, exact undo, leech and notifications | Covered at service level and against the real stack | `services/api/internal/review/handler_test.go`, `e2e/tests/real-stack-core.spec.ts` |
| Retry-safe review event | Covered against real Postgres | `services/api/internal/review/idempotency_integration_test.go` |
| Fifty offline reviews and exact queue drain after a committed/lost response | Covered in a real browser against the real API | `e2e/tests/real-stack-core.spec.ts` |
| Stats route and visual primitives | Covered with mock UI and real API state | `e2e/tests/journey-stats.spec.ts`, `e2e/tests/real-stack-core.spec.ts` |
| Settings tab accessibility | Covered with mock API | `e2e/tests/journey-settings-tabs.spec.ts` |
| Output landing, exercises, submission and empty states | Covered with mock UI and real API submission | `e2e/tests/journey-output.spec.ts`, `e2e/tests/real-stack-core.spec.ts` |

## Import, export, and library

| Flow | Status | Enforced evidence |
|---|---|---|
| Import UI tabs and format selection | Covered with mock API | `e2e/tests/journey-import.spec.ts` |
| Anki scheduling and Unicode round trip | Covered against real Postgres/API | `services/api/internal/importer/anki_integration_test.go`, `e2e/tests/real-stack-core.spec.ts` |
| Anki/Migaku/Yomitan/JPDB import and malformed archive rejection | Covered at service and real-stack levels | `services/api/internal/importer/handler_test.go`, `e2e/tests/real-stack-core.spec.ts` |
| JSON/CSV/APKG export structure and scheduling | Covered at service and real-stack levels; APKG media remains unshipped | `services/api/internal/export/export_test.go`, `e2e/tests/real-stack-core.spec.ts` |
| URL reader and TXT/SRT import | Covered at service and real-stack levels | `services/api/internal/library/handler_test.go`, `e2e/tests/real-stack-core.spec.ts` |
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
| Eleven API language codes tokenize and honor persisted knowledge; Vietnamese has no local dictionary entries | Covered locally and in the isolated stack | `services/nlp/tests/test_correctness.py`, `e2e/tests/real-stack-core.spec.ts` |
| Google Translation/TTS | Inconclusive when service-account credentials are unavailable |
| Anthropic contextual explanation | Inconclusive when API credentials are unavailable |
| Authenticated Netflix/Disney+/Prime/Crunchyroll/Viki live playback | Inconclusive without active provider sessions; recorded fixtures remain covered |
| Production register/create/review synthetic | Available but requires explicit production configuration | `scripts/synthetic.mjs` |

Adding a shipped flow requires an enforced test or an explicit `Partial` or
`Unverified` entry. Aspirational test filenames are not accepted as evidence.
