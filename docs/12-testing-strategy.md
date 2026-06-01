# Testing Strategy

> **Principle:** every claim of correctness must trace back to an automated
> assertion. Manual QA is not a release gate; features without tests in the
> matrix below ship behind an `experimental/` flag and are off in production.

This doc describes the sixteen layers (L0–L15) that together replace a
human beta tester. Each layer answers a specific failure mode and runs at a
specific cadence.

---

## What a beta tester catches that a unit suite does not

1. **Journeys** — a sequence of features used together (register → mine →
   review → see stats). Unit tests can't see "step 3 broke step 5."
2. **Drift** — Netflix renames a CSS class, Apple changes the PWA install
   rules, a customer's `.apkg` uses a schema variant. Code that was right
   yesterday is wrong today, with no commit between.
3. **Polish** — design rhythm, consistent spacing, missing empty states,
   AI-generic gradients, contrast failures. None of these break a unit test.

This strategy instruments all three.

---

## The layered map

| L  | Layer                       | Tool                                                                       | Catches                                                            | Gate                                          |
|----|-----------------------------|----------------------------------------------------------------------------|--------------------------------------------------------------------|-----------------------------------------------|
| 0  | Unit                        | vitest, `go test`, pytest                                                  | Pure logic regressions                                             | PR required                                   |
| 1  | Property-based              | fast-check, `testing/quick`, Hypothesis                                    | Edge cases not enumerated by example tests                         | PR required                                   |
| 2  | Mutation                    | Stryker, go-mutesting, mutmut                                              | Tests that exist but don't actually fail when behavior breaks      | Nightly; PR blocks when score regresses       |
| 3  | Integration                 | testcontainers Postgres                                                    | SQL, transactional state, real handler wiring                      | PR required                                   |
| 4  | API contract                | OpenAPI + schemathesis                                                     | Client ↔ server drift, undocumented status codes                   | PR required                                   |
| 5  | E2E web                     | Playwright × Chromium/Firefox/WebKit                                       | Full user journeys                                                 | PR required (matrix)                          |
| 6  | E2E extension               | Playwright with extension loaded                                           | Mining flow against recorded streaming SPA                         | PR required                                   |
| 7  | Visual regression           | Playwright `toHaveScreenshot`                                              | Unintended UI changes per component / per route                    | PR warning; reviewer approves new baselines   |
| 8  | Accessibility               | `@axe-core/playwright`                                                     | Keyboard traps, contrast, ARIA roles                               | PR required (zero serious violations)         |
| 9  | PWA + offline               | Playwright `context.setOffline(true)` + IndexedDB inspection               | Track 6 §6.4 acceptance — 50-review airplane mode                  | PR required                                   |
| 10 | Anki round-trip property    | Hypothesis-generated `.apkg`                                               | Scheduling preservation — the "drop-in" claim                      | PR required                                   |
| 11 | Streaming canary            | Recorded DOM fixtures + hourly live canary                                 | Disney+ / Prime / Netflix / Crunchyroll / Viki / YouTube DOM drift | Hourly cron; pages on regression              |
| 12 | Synthetic STT corpus        | Fixture utterances + golden diffs                                          | Transcription / WER / diff pipeline                                | PR required                                   |
| 13 | Performance budget          | Lighthouse CI, k6                                                          | Bundle bloat, p95 regression                                       | PR warning; nightly hard fail                 |
| 14 | LLM-as-judge polish         | Claude API + written rubric + Playwright screenshots                       | "Better design / UX / polished flows"                              | Nightly; release-blocking on score regression |
| 15 | Production synthetic        | Cron Playwright runner                                                     | "Does prod work *right now*?"                                      | 1-minute cadence                              |

---

## Gating policy

Every PR must pass **L0, L1, L3, L4, L5, L6, L8, L9, L10, L12**.

The nightly job runs **L2, L13, L14** and posts a status check to the
default branch; a regression there blocks the next release tag, not the
next PR. **L11** runs hourly. **L15** runs every minute.

Mutation score (L2) and polish score (L14) are tracked over time. The
public quality metric is *mutation score × polish score*, not coverage
percent — see "Why mutation testing is load-bearing" below.

---

## Layer-by-layer build

### L1 — Property-based

Three flavors, each rooted to a property the example tests don't cover:

- **`fast-check`** in `apps/extension/src/nlp/__tests__/property.test.ts`
  asserts that `wasmTokenize(s).map(t => t.surface).join("")` is a prefix of
  `s` for any UTF-8 input, and that re-tokenizing a re-joined output is
  idempotent.
- **Hypothesis** in `services/nlp/tests/test_property_en.py` generates
  random sentences from Tatoeba's English corpus and asserts that
  lemmatizing a known-irregular form always returns the canonical lemma.
  Strategy is exposed so failures shrink to a minimal example.
- **`testing/quick`** in `services/api/internal/fsrs/property_test.go` —
  for any (stability, difficulty, rating) tuple, asserts that
  `Schedule(p, cs, Easy)` returns a larger next-due interval than
  `Schedule(p, cs, Again)`. Catches monotonicity bugs.

### L2 — Mutation testing

Three tool stacks: Stryker (TS), go-mutesting (Go), mutmut (Python). The
nightly job posts a `mutation/score` GitHub status check; PRs whose diff
introduces files with score < 70% red-light. Surviving mutants are written
to `tests/mutation/surviving.json` so the next PR can attack them.

### L3 — Integration with testcontainers

`services/api/internal/db/testdb.go` exposes
`SetupPostgres(t *testing.T) *pgxpool.Pool`. It boots an ephemeral Postgres
container, runs every migration in `services/api/migrations/`, returns a
pool, and registers cleanup. Handler tests that currently mock `db = nil`
get migrated to call `SetupPostgres` instead. The unit-test layer keeps
its mocked variants for fast feedback; the integration layer is the
correctness gate.

### L4 — OpenAPI + schemathesis

`docs/openapi.yaml` is the source of truth for every `/v1` endpoint. The
TS client in `apps/web/src/lib/api.ts` is regenerated from it via
`openapi-typescript`; drift fails CI. **schemathesis** runs property-based
HTTP fuzz against the API container — generates random valid + invalid
payloads, asserts the server's responses conform to the schema.

### L5/L6 — Playwright E2E

`e2e/playwright.config.ts` declares three projects:

- `web-chromium`, `web-firefox`, `web-webkit` — full SvelteKit dev server
  against ephemeral Postgres + NLP, run journeys in `e2e/journeys/`.
- `extension-chrome`, `extension-firefox` — load the built extension into
  a real browser context, mine cards on the fixture SPA in
  `e2e/fixtures/streaming/`.

Every test calls `await injectAxe(page); await checkA11y(page)` before
asserting positive behavior. That's layer 8.

### L7 — Visual regression

Each route gets a screenshot per browser project, baselined in
`e2e/__screenshots__/`. The dark theme is forced via
`localStorage.setItem('theme', 'dark')` before each capture so dark/light
runs are deterministic. Diffs of more than 0.5% pixel difference fail the
job; reviewer accepts new baselines by running `pnpm test:visual --update`
in the PR's `gh-actions/` workflow run.

### L9 — PWA offline

`e2e/journeys/offline-review.spec.ts`:

```ts
await context.setOffline(true);
for (let i = 0; i < 50; i++) {
  await page.keyboard.press(' ');     // flip
  await page.keyboard.press('3');     // Good
}
// Assert queue depth = 50 in IndexedDB
const queueLen = await page.evaluate(() => /* read carve_offline */);
expect(queueLen).toBe(50);

await context.setOffline(false);
await page.waitForFunction(/* queue length === 0 */);
const serverCount = await apiHelper.reviewEventCount(userId);
expect(serverCount).toBe(50);
```

### L10 — Anki round-trip

**This is the single most important test for the drop-in claim.**

`services/api/internal/importer/property_test.go` does:

1. Generate a random `.apkg` archive containing 10–500 notes with random
   `(ivl, ease, due, lapses, reps)` distributions.
2. Import via `Handler.ImportAnki`.
3. Re-export.
4. Assert that scheduling fields survive a structural mapping —
   `(stability, difficulty, due, lapses, reps)` reproduces deterministically
   across two import/export cycles.

Until the importer's actual scheduling-preservation code is written (it
currently sets every card to `fsrs_state='new'`), this test is the
forcing function. See `internal/importer/anki_sched.go` for the SM-2 → FSRS
mapping (Stability ≈ `ivl` days, Difficulty derived from `ease`).

### L11 — Streaming canary

Two complementary mechanisms:

- **Fixture replay** — `apps/extension/src/content/video/__tests__/fixtures/`
  contains one `.html` per platform: a snapshot of the player DOM around
  an active cue. Vitest builds the DOM via `jsdom`, calls `hook.mount()`,
  asserts that the overlay receives a `{ text, startMs, endMs }` cue.
- **Live canary** — `e2e/canary/streaming.spec.ts`, run hourly via GitHub
  Actions `schedule` cron, logs into a test account per platform, opens a
  known episode, asserts the hook fires. Credentials live in 1Password and
  are pulled by the CI job at runtime. Brittle by nature; that brittleness
  is the point — when Netflix renames a class, the canary pages someone
  within the hour.

### L12 — STT corpus

`tests/stt-corpus/` holds `.txt` golden pairs: reference + hypothesis.
The `transcribe.go` diff/WER is asserted against the expected results in
`internal/output/transcribe_corpus_test.go`. End-to-end: a Playwright test
mocks `MediaRecorder` with a pre-recorded blob, drives the shadowing page,
asserts the diff renders the right colored tokens.

### L13 — Lighthouse + k6

`.github/workflows/perf.yml` runs Lighthouse against the deployed preview
URL on every PR and asserts the PWA, accessibility, performance, and
best-practices categories ≥ 90. k6 fires 1 RPS at `/v1/cards` and 5 RPS at
`/v1/review/session` for one minute; p95 must stay under 250 ms.

### L14 — LLM-as-judge for polish

`scripts/polish-review.ts` captures one screenshot per route via
Playwright, ships them and `docs/13-design-rubric.md` to the Claude API,
and parses the per-route 1–5 score back into `polish-scores.json`. CI
fails when any route's score regresses by ≥ 0.5 vs. main.

The rubric is the load-bearing artifact:

- design tokens used (not inline hex codes)
- consistent spacing rhythm (8/12/16/24px scale)
- typography hierarchy clear (no two H1s)
- empty states present and informative
- no generic AI-aesthetic gradients
- contrast ≥ WCAG AA
- interactive affordances visible (focus rings, hover states)

Changes to the rubric are treated like API changes — versioned, reviewed,
and tied to an entry in `docs/14-rubric-changelog.md`.

### L15 — Production synthetic monitoring

`scripts/synthetic.ts` runs every minute (Datadog cron or a single
long-lived worker): registers a throwaway user, mines a card on a public
fixture page, submits a review, asserts retrieval. Posts pass/fail to a
dashboard. Failures page the on-call within five minutes.

---

## Why mutation testing is load-bearing

Coverage tells you which lines ran, not whether the assertions noticed
when those lines did the wrong thing. A test suite at 95% coverage can
have a 20% mutation score — meaning four out of five behavior changes
silently pass. Mutation testing audits the suite itself.

Track *mutation score per package* over time. Make it the public quality
metric. Target 70% killed mutants per package, ratchet up by 5 points per
quarter. Surviving mutants are the next sprint's backlog.

---

## Build sequence

1. **Week 1** — L3 + L10 + L1.
   These three together retire the two manual checks that block the
   "drop-in" claim. Anki round-trip is property-based against a real DB;
   tokenizer/FSRS properties are property-based against pure functions.

2. **Week 2** — L5 + L6 + L8 + L9.
   The Playwright matrix with a11y + offline pass earns the public-beta
   gate. Add L12 alongside since it's an extension of L5.

3. **Week 3** — L4 + L7 + L11 + L13.
   Contract drift, visual regression, streaming-platform drift, perf
   budget. Now the per-PR feedback loop catches almost every class of
   regression.

4. **Week 4** — L2 + L14 + L15.
   Mutation testing (proves the suite catches what it claims to), polish
   review (proves the UI doesn't regress in look), production synthetic
   (proves prod still works).

After week 4 there is no human in the verification loop. Anything that
ships passed every layer; anything that breaks pages someone before a
user sees it.
