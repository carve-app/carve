# Testing Strategy

Last verified: 2026-06-27. This document describes tests that exist and are
enforced. Planned work is labelled; filenames are checked by
`pnpm test:flow-docs`.

## Gates

| Layer | Current proof | Enforcement |
|---|---|---|
| Formatting/lint | ESLint flat config for Svelte/TypeScript; `go vet` | Every PR |
| Type safety | `svelte-check`, `tsc --noEmit` | Every PR |
| Unit/property | Vitest, Go race tests, pytest/Hypothesis, Rust tests | Every PR |
| Targeted coverage | Web offline persistence and extension storage, minimum 80% lines | Every PR |
| SQL integration | Testcontainers Postgres, all migrations, importer/review handler tests | Every PR |
| API contract | Runtime route/OpenAPI parity plus authenticated Schemathesis | Every PR |
| Web E2E | Playwright Chromium, Firefox, WebKit, and mobile WebKit; accessibility checks | Every PR |
| Extension E2E | Built Chrome package journeys; real Firefox `web-ext` content-script smoke | Every PR |
| Offline | 50 queued reviews, queue drains to zero, 50 unique server submissions | Every PR |
| Build/package | Static SvelteKit SPA at `apps/web/build`; Chrome/Firefox/Safari bundles | Every PR |
| Dependencies | `pnpm audit --audit-level moderate` (the audit currently resolves all severities) | Every PR |
| Mutation | Targeted Stryker plus go-mutesting and mutmut; 65% blocking threshold | Nightly |
| Backup/restore | Fresh and staged migrations followed by PostgreSQL logical restore and data/history comparison | Every PR |
| Performance | Blocking Lighthouse assertions and authenticated k6 script | Nightly/manual full gate |
| Live provider | Public YouTube canary | Hourly |

The authoritative flow-to-test map is [15-flow-inventory.md](15-flow-inventory.md).

## Local commands

```bash
pnpm install --frozen-lockfile
pnpm lint
pnpm typecheck
pnpm test:coverage
pnpm --filter @carve/web exec stryker run
pnpm --filter @carve/extension exec stryker run
pnpm build
pnpm test:flow-docs
pnpm audit --audit-level moderate

(cd services/api && go vet ./... && go test -race -count=1 ./...)
(cd services/nlp && .venv/bin/python -m pytest -q)
(cd services/media && go vet ./... && go test -race -count=1 ./...)
(cd apps/extension/wasm-src/ja-tokenizer && cargo test --locked)
(cd apps/extension/wasm-src/ko-tokenizer && cargo test --locked)
(cd apps/extension/wasm-src/zh-tokenizer && cargo test --locked)

# Docker, real API/NLP/media/Postgres/Mailpit, and Chromium
./scripts/test-real-stack.sh

# Authenticated OpenAPI conformance (requires an isolated running API and token)
schemathesis run docs/openapi.yaml --url http://127.0.0.1:8080 \
  --phases coverage,fuzzing --max-examples 25 \
  --exclude-path-regex '^/v1/billing/' --exclude-operation-id deleteMe \
  --checks status_code_conformance,content_type_conformance \
  --header "Authorization: Bearer $ACCESS_TOKEN"

# Authenticated semantic load proof (API must expose test verification tokens)
docker run --rm --add-host=host.docker.internal:host-gateway \
  -v "$PWD/tests/perf:/scripts:ro" grafana/k6:latest run /scripts/load.js \
  -e API_BASE=http://host.docker.internal:8080
```

The fast browser suite uses `e2e/mock-server.cjs`. That server rejects unknown
routes with 501 and validates the requests it supports; it is a deterministic
UI test double, not evidence of backend integration. `test-real-stack.sh` is
the integration proof and uses isolated volumes and non-default host ports.

## What each important suite proves

- `services/api/internal/review/idempotency_integration_test.go`: duplicate
  client event IDs return the stored scheduling response and create one event.
- `apps/web/src/lib/__tests__/offline*.test.ts` and
  `e2e/tests/pwa-offline.spec.ts`: IndexedDB failure is visible, persisted
  events replay, and a 50-event queue drains exactly once.
- `e2e/tests/real-stack-core.spec.ts`: six proof journeys against the actual
  services: verification/login/refresh/logout/reset mail; real built-UI
  onboarding/cards/review/bulk/reader behavior; persisted knowledge, lookup and
  tokenization for all eleven API language codes; starter-deck and grammar
  persistence; cards/media/bulk/bury/review/undo/leech/output/stats;
  media-preserving import/export/readers; and 50 real browser IndexedDB reviews
  with a committed response deliberately lost before retry.
- `e2e/tests/real-stack-restart.spec.ts`: a committed event replays byte-for-byte
  and remains exactly once after the real API process restarts.
- `e2e/extension.test.js`: packaged Chrome annotation, ruby preservation,
  lookup and mining.
- `e2e/firefox-extension-smoke.cjs`: a built Firefox package is installed by
  `web-ext` and its content/background path reaches the NLP endpoint.
- `e2e/tests/extension-streaming.spec.ts`: six recorded streaming-platform DOM
  integrations and progressive YouTube caption behavior.
- `e2e/video-mining.test.js`: real-stack Chrome video media capture/mining.
- `services/api/cmd/api/router_test.go`: public/auth boundaries, metrics auth,
  and exact registered-route/OpenAPI parity.

## Cadence and failure policy

Ordinary CI has no schedule-only jobs. `.github/workflows/scheduled-quality.yml`
runs the hourly YouTube provider check only on the hourly expression, and the
mutation/Lighthouse work only on the nightly expression. Contract, mutation,
performance and provider commands do not suppress failures.

Live results use only `passed`, `failed`, or `inconclusive`. Missing credentials
or a missing authenticated browser session is `inconclusive`, never `passed`.
The dated audit report records the environment and reason.

## Known limits

- Safari output is manifest/build-validated only. Runtime remains unverified
  until a native Safari Web Extension wrapper and signing project exist.
- Visual snapshots cover representative public routes, not every authenticated
  state. UI coverage beyond the changed persistence modules is still below the
  desired 80% line threshold.
- Stryker's blocking scope is the changed offline/storage durability code. A
  diagnostic whole-web run scored 9.43%, so expanding mutation coverage across
  the legacy API client and stores remains prioritized work rather than a
  claimed pass.
- Real AnkiConnect requires an Anki process and remains a manual/full-gate
  integration; transport behavior is automated.
- APKG import/export preserves attached image/audio bytes, Unicode, and
  scheduling. EPUB remains explicitly unshipped.
- Official starter decks are currently seeded only for Japanese and English.
  Onboarding explicitly reports the unavailable case for the other selectable
  languages and no longer claims that a deck was subscribed.
- Production synthetic, authenticated streaming services, Google Cloud and
  Anthropic checks require explicit credentials and are never inferred from
  mock or fixture success.
