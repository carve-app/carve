# Implementation Roadmap

> **Historical planning doc — superseded.** For what is actually built and
> verified, see [STATUS.md](STATUS.md). This file is kept as the original phased
> plan and is no longer maintained against the codebase.

---

## Guiding Principles

- **Ship working, not complete.** Each phase must produce a functional product that real users can use.
- **Quality gate before marketing.** No public launch until the Japanese NLP correctness test suite passes 100% on the documented failure cases.
- **Open from day one.** The repository is public from the first commit. Transparency builds trust.
- **User data is sacred.** Data export must work before we charge a single user.

---

## Phase 0 — Foundation (Weeks 1–6)

Goal: Working monorepo, CI/CD, infrastructure, and verified NLP core.

### Deliverables

- [ ] Monorepo structure (pnpm workspaces): `apps/web`, `apps/extension`, `services/api`, `services/nlp`, `services/media`
- [ ] Docker Compose dev environment (postgres, redis, minio, all services)
- [ ] GitHub Actions CI: lint, type-check, test, build on every PR
- [ ] PostgreSQL schema migrations (Goose or golang-migrate)
- [ ] Basic auth: register, login, JWT, refresh
- [ ] Japanese NLP correctness test suite (100 hand-verified cases)
- [ ] SudachiPy server-side tokenizer passing 100% of test suite
- [ ] Rust WASM Japanese tokenizer compiling and passing same suite
- [ ] JMdict imported into PostgreSQL
- [ ] `/nlp/tokenize` and `/nlp/lookup` API endpoints working
- [ ] Tatoeba example sentences imported

### Success Criteria

The Japanese tokenizer correctly handles all documented Migaku failure cases. If it doesn't, do not proceed.

---

## Phase 1 — MVP Extension (Weeks 7–14)

Goal: Usable Chrome extension for Japanese, connected to live API.

### Deliverables

- [ ] Chrome extension: manifest, service worker, content script scaffold
- [ ] WASM tokenizer loaded in content script (Japanese)
- [ ] Word popup: definition, reading, frequency rank
- [ ] Furigana rendering on hover (correctly aligned)
- [ ] Pitch accent display (from pre-loaded OJAD data)
- [ ] User vocabulary sync to IndexedDB
- [ ] Word status display in popup (unknown/learning/known)
- [ ] "Ignore" button (mark word as known without card)
- [ ] Basic card mining: one-click capture → POST /cards
- [ ] Card stored in PostgreSQL; viewable in simple web UI
- [ ] Extension toolbar popup: cards due count, link to web review
- [ ] Immersion time tracker (auto-log reading minutes)
- [ ] Netflix subtitle hook (Japanese)
- [ ] YouTube subtitle hook (Japanese)
- [ ] 10 internal beta testers

### Success Criteria

A tester can watch a Japanese Netflix show, look up any word, see correct furigana, mine a card, and have it appear in the review queue. Zero incorrect furigana reports from testers.

---

## Phase 2 — Core Review System (Weeks 15–22)

Goal: Full SRS review cycle with FSRS-6, web UI.

### Deliverables

- [ ] FSRS-6 implementation in Go (server-side)
- [ ] `POST /review/events` endpoint (submit ratings, update card state)
- [ ] `GET /review/session` endpoint (interleaved queue)
- [ ] Anti-similarity desimilarizer in queue builder
- [ ] Review UI in web app: flip card, rate, next
- [ ] Card stats displayed (S, D, R per card)
- [ ] Interval preview under each rating button
- [ ] Workload forecast (GET /review/forecast → 14-day chart)
- [ ] Offline review: cards cached in IndexedDB, sync on reconnect
- [ ] Leech detection and suspension with user notification
- [ ] FSRS parameter display in settings
- [ ] Retention target slider with workload preview
- [ ] Audio playback on cards (fetch from Forvo / JapanesePod101)
- [ ] Screenshot attachment for video-mined cards
- [ ] Pre-built JLPT N5 and N4 decks (seeded)
- [ ] Deck browser (public decks)
- [ ] Deck subscription (add all deck cards to user's queue)
- [ ] Data export: full JSON export working
- [ ] Billing integration (Stripe): Free, Learner, Pro tiers

### Success Criteria

Internal testers complete 100 review sessions without hitting a scheduling bug. Retention rate measured at 88–92% across testers (FSRS-6 target).

---

## Phase 3 — Public Beta (Weeks 23–32)

Goal: Open public beta with Japanese support. Start collecting real user data for FSRS optimizer.

### Deliverables

- [ ] Firefox extension published to AMO
- [ ] Safari Web Extension wrapper and submission to App Store
- [ ] Comprehension score overlay (toggle in extension)
- [ ] Color-coded frequency bands on all content
- [ ] `/nlp/score-content` endpoint (score a URL)
- [ ] Library: save URLs, view comprehension %, sorted list
- [ ] Stats dashboard: immersion time, known words, retention, streak
- [ ] Word count snapshot job (daily)
- [ ] Known-word growth chart (time series)
- [ ] FSRS optimizer (background job, triggers at 400+ reviews)
- [ ] User-facing optimizer: "Optimize my parameters" button + progress
- [ ] Community deck sharing (make own deck public, rate decks)
- [ ] Mobile PWA: review sessions work offline on mobile
- [ ] Public launch blog post + show HN

### Success Criteria

500 DAUs within 4 weeks of launch. NPS > 40 from early survey. Zero P0 bugs open for > 48 hours.

---

## Phase 4 — Chinese & Korean + Output (Weeks 33–44)

Goal: Add Mandarin Chinese and Korean. Introduce output practice features.

### Deliverables

**Chinese (Mandarin):**
- [ ] CEDICT imported into PostgreSQL
- [ ] jieba-rs WASM tokenizer (Chinese)
- [ ] Pinyin annotation with tone diacritics
- [ ] Traditional/Simplified toggle
- [ ] Tone color-coding (optional: 4 tones → 4 colors)
- [ ] Chinese correctness test suite (100 cases)

**Korean:**
- [ ] Korean WASM tokenizer
- [ ] Korean dictionary (KDE4 + custom)
- [ ] Korean correctness test suite
- [ ] Particle parsing display

**Output Practice:**
- [ ] Writing exercise: given words, write a sentence
- [ ] AI feedback on writing (Claude API: grammar, vocabulary, naturalness)
- [ ] Cloze deletion exercises generated from mined sentences
- [ ] Shadowing queue: audio plays, user transcribes
- [ ] Speaking drill: user records → speech-to-text → diff against expected
- [ ] Output exercises linked to SRS: recently mined words surface in output queue

### Success Criteria

Chinese and Korean test suites pass at 100%. At least 100 users actively using output features daily.

---

## Phase 5 — European Languages + Mobile Native (Weeks 45–56)

Goal: Spanish, French, German, Portuguese, Italian. Native mobile apps.

### Deliverables

**European Languages:**
- [ ] spaCy models integrated for ES, FR, DE, PT, IT
- [ ] Latin-script WASM tokenizer (rule-based, no ML)
- [ ] Frequency lists for each language (SUBTLEX corpora)
- [ ] Dictionary data (dict.cc, Wiktionary) for each language
- [ ] Correctness test suites per language

**Native Mobile:**
- [ ] iOS app (SwiftUI) — full feature parity with PWA
- [ ] Android app (Jetpack Compose) — full feature parity with PWA
- [ ] Reader mode for imported text (EPUB support)
- [ ] Video player with subtitle overlay (for locally stored content)
- [ ] Background audio immersion logger

**Platform:**
- [ ] Kubernetes auto-scaling configured and tested
- [ ] Multi-region deployment (US + EU)
- [ ] GDPR data deletion flow
- [ ] SOC 2 Type 1 audit initiated

---

## Phase 6 — Intelligence Layer (Weeks 57–72)

Goal: Content recommendations, AI-powered features at scale.

### Deliverables

- [x] Content discovery engine: given user vocab, recommend URLs (`/v1/discover/feed`)
- [x] NHK Easy + Watanoc integrations (Japanese reading); Satori Reader deferred (paywalled, no public feed)
- [ ] YouTube channel / playlist tracker (auto-add new videos to library) — deferred: needs YouTube Data API key
- [x] Grammar pattern library: 30 N5/N4/N3 JA patterns tracked alongside vocab (`services/nlp/carve_nlp/grammar_ja.py`)
- [x] Grammar difficulty in comprehension score: `/tokenize` returns `grammar_pct` + `unknown_patterns`
- [x] Sentence similarity detection: `/v1/cards/find-similar` (char-trigram Jaccard)
- [x] Smart sentence selection: `/v1/nlp/select-sentence` picks the best i+1 candidate from a context window
- [x] Personalized deck generation: `/v1/decks/generate` builds a deck from recent library reading
- [x] Weekly learning report email: `/v1/reports/weekly` + Monday 08:00 UTC SMTP cron
- [ ] Teacher/classroom mode (for small-group use) — deferred to Phase 7

**Phase 6 closed 2026-06-01** with two deliverables explicitly carried forward (YouTube tracker and Teacher mode) as their own scope items.

---

## Ongoing: Never Stop

These are perpetual engineering investments, not phases:

- **NLP accuracy**: continuous improvement; correctness test suites grow monthly
- **Dictionary quality**: community corrections, user-reported errors
- **FSRS research**: track FSRS paper updates; implement new versions when stable
- **Platform integrations**: new streaming services, news sites, reading apps
- **Security**: quarterly pen tests, dependency audits
- **Performance**: p99 latency targets for all API endpoints (< 200ms for lookup)

---

## Milestone Summary

| Milestone | Target Date | Key Outcome |
|---|---|---|
| Phase 0 complete | Week 6 | NLP correctness proven |
| Phase 1 complete | Week 14 | Usable extension (internal beta) |
| Phase 2 complete | Week 22 | Full review cycle working |
| Public beta launch | Week 32 | 500 DAU, revenue starts |
| Chinese + Korean | Week 44 | 3-language platform |
| European languages | Week 56 | 8 languages, mobile native |
| Intelligence layer | Week 72 | Content recommendations, AI features |

---

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| NLP accuracy below bar for launch | Medium | Critical | Phase 0 gate: do not ship until test suite passes |
| Streaming platform (Netflix) blocks subtitle hook | High | High | Content script approach is CSS-level; legal review; alternative: user-uploaded SRT files |
| FSRS-6 licensing conflict | Low | Low | FSRS is MIT licensed; open-spaced-repetition org explicitly supports third-party use |
| Self-hosted users undermine business | Low | Medium | Core is AGPL: any improvements must be contributed back; managed service has convenience value |
| Chrome MV3 removes required APIs | Low | High | Design extension to work without `webRequest`; use `declarativeNetRequest` throughout |
| Competitor copies open-source code | High | Low | AGPL license requires open-sourcing derivatives; competitive moat is execution and community |
| Burn rate exceeds revenue | Medium | High | Keep team small; charge from Phase 3; self-hosted provides no support cost |
