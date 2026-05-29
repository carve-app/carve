# System Architecture

## Overview

Carve is a distributed system with five major subsystems that communicate via well-defined interfaces. The design prioritizes local-first processing (NLP runs in-browser via WASM for privacy and latency), with a backend that handles persistence, sync, and heavier computation.

```
┌─────────────────────────────────────────────────────────────────┐
│                        CLIENT LAYER                             │
│                                                                 │
│  ┌───────────────────┐    ┌──────────────────┐                 │
│  │  Browser Extension│    │   Web App (SPA)  │                 │
│  │ (WebExtension API)│    │   (SvelteKit)    │                 │
│  │                   │    │                  │                 │
│  │ ┌───────────────┐ │    │  Review · Stats  │                 │
│  │ │  NLP Engine   │ │    │  Library · Output│                 │
│  │ │   (WASM)      │ │    │                  │                 │
│  │ └───────────────┘ │    └──────────────────┘                 │
│  │                   │                                         │
│  │ ┌───────────────┐ │    ┌──────────────────┐                 │
│  │ │  Dict Cache   │ │    │  Mobile App      │                 │
│  │ │  (IndexedDB)  │ │    │  (PWA / Native)  │                 │
│  │ └───────────────┘ │    └──────────────────┘                 │
│  └───────────────────┘                                         │
└───────────────────────────┬─────────────────────────────────────┘
                            │ HTTPS / WSS
┌───────────────────────────▼─────────────────────────────────────┐
│                        API GATEWAY                              │
│              (Caddy reverse proxy + rate limiting)              │
└───────────────────────────┬─────────────────────────────────────┘
                            │
        ┌───────────────────┼────────────────────┐
        │                   │                    │
┌───────▼──────┐  ┌─────────▼──────┐  ┌─────────▼──────┐
│  Core API    │  │  NLP Service   │  │  Media Service │
│  (Go)        │  │  (Python/Rust) │  │  (Go)          │
│              │  │                │  │                │
│ Auth · Users │  │ Tokenization   │  │ Audio fetch    │
│ Cards · SRS  │  │ Dict lookup    │  │ Screenshot cap │
│ Decks · Sync │  │ Difficulty est.│  │ Storage proxy  │
│ Immersion log│  │ Grammar parse  │  │                │
└──────┬───────┘  └────────┬───────┘  └────────┬───────┘
       │                   │                    │
┌──────▼───────────────────▼────────────────────▼───────┐
│                   DATA LAYER                           │
│                                                        │
│  ┌──────────────┐  ┌──────────────┐  ┌─────────────┐ │
│  │  PostgreSQL  │  │    Redis     │  │  S3-compat  │ │
│  │              │  │              │  │  (MinIO /   │ │
│  │ Users, cards │  │ Session cache│  │   R2)       │ │
│  │ reviews,     │  │ Rate limits  │  │             │ │
│  │ vocabulary,  │  │ NLP cache    │  │ Audio files │ │
│  │ immersion log│  │ Dict lookups │  │ Screenshots │ │
│  └──────────────┘  └──────────────┘  │ Deck assets │ │
│                                       └─────────────┘ │
│  ┌──────────────────────────────────────────────────┐ │
│  │           Dictionary Store                       │ │
│  │    (SQLite files bundled with clients +          │ │
│  │     PostgreSQL for server-side lookup)           │ │
│  │                                                  │ │
│  │  JMdict (JP-EN), CEDICT (ZH-EN), Wiktionary,   │ │
│  │  KDE4 dict (KO-EN), dict.cc (DE, FR, etc.)     │ │
│  └──────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────┘
```

---

## Subsystem Descriptions

### 1. Browser Extension

The extension is the primary interaction surface for immersion learners. It runs entirely in the user's browser and processes content locally for speed and privacy.

**Components:**

```
extension/
├── manifest.json           # WebExtension manifest v3
├── background/
│   └── service-worker.ts   # Auth, sync queue, badge updates
├── content/
│   ├── injector.ts         # Page analysis entry point
│   ├── overlay.ts          # Word highlighting and coloring
│   ├── popup.ts            # Word lookup popup UI
│   ├── subtitle-hook.ts    # Netflix/YouTube subtitle interception
│   └── time-tracker.ts     # Active reading time measurement
├── nlp/
│   ├── wasm/               # Compiled WASM tokenizer modules per language
│   ├── tokenizer.ts        # WASM bridge: text → tokens
│   ├── dict-cache.ts       # IndexedDB dictionary cache
│   └── scorer.ts           # Comprehension score calculation
├── popup/
│   └── popup.html/.ts      # Extension toolbar popup (stats, settings)
└── options/
    └── options.html/.ts    # Settings page
```

**Data flow (word lookup):**

```
User hovers over word
        │
        ▼
content/injector.ts detects mouse event
        │
        ▼
nlp/tokenizer.ts: segment surrounding text (WASM, <5ms)
        │
        ▼
nlp/dict-cache.ts: lookup in IndexedDB local dictionary
        │
  ┌─────┴──────┐
  │ Cache hit  │ Cache miss
  │            │
  ▼            ▼
Show popup    API: POST /nlp/lookup
              │
              ▼
           Show popup + cache result in IndexedDB
```

**Subtitle interception (video content):**

```
subtitle-hook.ts patches MutationObserver on video player DOM
        │
        ▼
Each subtitle text node is segmented by WASM tokenizer
        │
        ▼
Tokens matched against user's known-word set (IndexedDB)
        │
        ▼
Subtitle words color-coded in-place (no DOM replacement, CSS ::before)
        │
        ▼
User clicks word → popup appears → optional card mine
```

---

### 2. Web App

SvelteKit single-page application with server-side rendering for initial load performance. Authentication via JWT with refresh tokens.

**Route structure:**

```
/                   Landing / marketing
/app/               Authenticated app root
/app/review         Daily SRS review session
/app/vocabulary     Word list, search, filter
/app/decks          Deck browser (pre-built + community + custom)
/app/library        Saved content items
/app/stats          Immersion dashboard and analytics
/app/output         Production exercises
/app/settings       User preferences, language config
/app/export         Data export tools
```

**State management:**
- Local SRS state cached in IndexedDB for offline review
- Server sync via background push (WebSocket when app is open, background sync API when closed)
- Optimistic updates for card reviews (fire-and-forget with retry queue)

---

### 3. Core API (Go)

RESTful API with WebSocket for real-time sync. Stateless; all session state in JWT or Redis.

**Key responsibilities:**
- User auth (JWT + refresh, OAuth via Google/GitHub)
- Card CRUD + bulk operations
- SRS scheduling (FSRS-6 algorithm, runs server-side for multi-device consistency)
- Deck management (create, share, clone, rate)
- Immersion log entries
- Vocabulary knowledge tracking
- Content registry (URL → vocabulary profile)
- Data export generation

**Service boundaries:**
- Core API never calls the NLP service synchronously in request path (too slow). NLP results are pre-computed or fetched asynchronously.
- Media operations delegate to the Media Service.

---

### 4. NLP Service (Python + Rust extensions)

Provides tokenization, morphological analysis, dictionary lookup, and difficulty scoring for all supported languages. Python for orchestration; Rust for performance-critical tokenization.

**Language-specific tokenizers:**

| Language | Tokenizer | Notes |
|---|---|---|
| Japanese | SudachiPy (via Python) + Rust bridge | Three granularity levels (A/B/C) |
| Chinese | Jieba (zh-cn) / HanLP | Simplified + Traditional |
| Korean | KoNLPy (Okt or Mecab-ko) | Particle splitting |
| Spanish/French/German/etc. | spaCy (language-specific models) | Fast, good POS tagging |

**WASM modules (for in-browser use):**
- Each language tokenizer is compiled to WASM using a lightweight Rust implementation
- WASM modules are ~2–5 MB per language, cached in the browser after first load
- In-browser tokenization is used for real-time annotation; server-side for pre-indexing content

---

### 5. Media Service (Go)

Handles fetching and caching of audio and image assets for cards.

- **Audio**: Fetches pronunciation audio from Forvo, JapanesePod101 (licensed), or synthesizes via TTS (Google TTS or open-source Piper TTS)
- **Screenshots**: Captures video frames at the timestamp of card mining via a headless browser or client-side canvas API
- **Storage**: Assets stored in S3-compatible object storage (MinIO for self-hosted, Cloudflare R2 for managed)
- **Deduplication**: Content-addressed storage (SHA-256 hash) prevents duplicate audio files per word

---

## Cross-Cutting Concerns

### Authentication & Authorization

```
User → POST /auth/login → JWT (15min) + Refresh token (30 days)
                         │
                         ├── JWT in Authorization header for all API calls
                         └── Refresh token in HttpOnly cookie

OAuth flow: Google/GitHub → callback → create/link account → issue JWT
```

### Sync Architecture

The system uses a **last-write-wins CRDT-adjacent approach** for card state:

- Each card review creates an immutable event record
- On sync, events are merged by timestamp; conflicts resolved by latest event wins
- Client is the source of truth for review events; server validates and persists
- Offline support: reviews queue in IndexedDB, sync when connection restored

### Caching Strategy

| Layer | Technology | TTL | Contents |
|---|---|---|---|
| Browser (extension) | IndexedDB | 30 days | Dictionary lookups, user vocabulary |
| Browser (web app) | IndexedDB | 7 days | Due cards for offline review |
| API | Redis | 24 hours | User vocabulary set (for comprehension scoring) |
| API | Redis | 1 hour | Dictionary lookups |
| API | Redis | 5 min | NLP tokenization results |
| CDN | Cloudflare | 1 year | Static assets, WASM modules |

### Content Security & Privacy

- The extension reads page content only when the user has it enabled and is on a target-language domain
- No page content is sent to servers unless the user explicitly requests server-side NLP (for languages without WASM tokenizer)
- Review history and vocabulary are encrypted at rest (AES-256)
- Users can delete all data with a single action; deletion is immediate and complete
- No analytics tracking injected into the extension; opt-in telemetry only

### Observability

```
Metrics: Prometheus + Grafana
Traces:  OpenTelemetry → Tempo
Logs:    Structured JSON → Loki
Alerts:  PagerDuty-compatible webhooks

Key metrics to track:
- NLP tokenization latency (p50, p99)
- Dictionary cache hit rate
- SRS review completion rate
- Sync conflict rate
- API error rate by endpoint
```

---

## Deployment

### Managed Cloud (primary)

```
Kubernetes (GKE or EKS)
├── Namespace: carve-prod
│   ├── Deployment: core-api (3 replicas min)
│   ├── Deployment: nlp-service (2 replicas, GPU optional)
│   ├── Deployment: media-service (2 replicas)
│   ├── StatefulSet: postgresql (primary + 1 replica)
│   ├── Deployment: redis (Sentinel mode)
│   └── CronJob: srs-scheduler (runs every hour, pre-computes due cards)
├── Namespace: carve-monitoring
│   └── Prometheus, Grafana, Loki, Tempo
└── Ingress: Caddy (TLS, rate limiting, compression)
```

### Self-Hosted (Docker Compose)

```yaml
# docker-compose.yml (simplified view)
services:
  api:        image: ghcr.io/carve-app/core-api
  nlp:        image: ghcr.io/carve-app/nlp-service
  media:      image: ghcr.io/carve-app/media-service
  web:        image: ghcr.io/carve-app/web
  postgres:   image: postgres:16
  redis:      image: redis:7-alpine
  minio:      image: minio/minio
  caddy:      image: caddy:2
```

Single `docker compose up` gets a fully functional instance. Configuration via `.env` file. Self-hosters receive the same features as paid users; they are responsible for their own infrastructure.

---

## Technology Stack Summary

| Layer | Technology | Rationale |
|---|---|---|
| Web App | SvelteKit + TypeScript | Lightweight, fast SSR, excellent DX |
| Browser Extension | TypeScript + WebExtension API | Cross-browser, type-safe |
| In-browser NLP | Rust → WASM | Near-native speed, privacy (no server call) |
| Core API | Go | High throughput, low latency, small binary |
| NLP Service | Python + Rust extensions | Best NLP library ecosystem; Rust for hot paths |
| Database | PostgreSQL 16 | Proven, JSON support, full-text search |
| Cache | Redis 7 | Session, rate limiting, NLP result cache |
| Object Storage | S3-compatible (Cloudflare R2 / MinIO) | Cheap egress, simple API |
| SRS Algorithm | FSRS-6 | State of the art; open reference implementation |
| Reverse Proxy | Caddy | Automatic TLS, clean config |
| Container Orch | Kubernetes (prod) / Docker Compose (self-host) | Standard; good tooling |
| CI/CD | GitHub Actions | Free for public repos; caches well |
| Monitoring | Prometheus + Grafana + OpenTelemetry | Open source; self-hostable |
