# Carve

> Cut through the noise. Carve fluency from real content.

Carve is a science-grounded language learning platform built for serious immersion learners. It replaces the fragmented toolchain of Anki + Yomitan + Migaku with a single, open, cross-platform system that is accurate, transparent, and genuinely grounded in second language acquisition research.

---

## Documentation Index

| Document | Description |
|---|---|
| [Research: Migaku & SLA Science](docs/01-research.md) | Market analysis, community pain points, SLA theory synthesis |
| [Product Design](docs/02-product-design.md) | Core philosophy, feature set, UX principles |
| [System Architecture](docs/03-architecture.md) | High-level system design, component map |
| [Data Models](docs/04-data-models.md) | Database schemas, entity relationships |
| [API Design](docs/05-api-design.md) | REST + WebSocket API specifications |
| [Browser Extension](docs/06-browser-extension.md) | Extension architecture, NLP in-browser |
| [NLP Pipeline](docs/07-nlp-pipeline.md) | Language processing, tokenization, difficulty scoring |
| [SRS System](docs/08-srs-system.md) | FSRS-6 implementation, review scheduling |
| [Implementation Roadmap](docs/09-roadmap.md) | Phased delivery plan |

---

## What Makes Carve Different

| Pain Point (Migaku / existing tools) | Carve's Answer |
|---|---|
| Chrome-only extension | Cross-browser (Chrome, Firefox, Safari) via WebExtension API |
| Inaccurate morpheme parsing and furigana | Language-specific WASM tokenizers with correctness test suites |
| Misleading translations | Layered dictionary system + confidence scores + human-verified corpus |
| Opaque SRS scheduling | FSRS-6 with visible stability/retrievability metrics per card |
| No output practice | Integrated writing and speaking drills built from mined vocabulary |
| No immersion tracking | Built-in time tracker: reading minutes, listening minutes, active review |
| Subscription-only, data locked in | Core is open-source; all user data exportable as standard JSON/CSV |
| Steep learning curve | Progressive onboarding with sensible defaults; power features opt-in |
| Poor mobile experience | Mobile-first PWA + native companion apps |
| No i+1 content matching | Real-time comprehension score on any page; content recommendations |
| Pricing opacity / lifetime price hikes | Transparent pricing; self-hostable for free |

---

## Quick Start (Development)

```bash
# Prerequisites: Node 22+, Rust (for WASM build), Go 1.23+, PostgreSQL 16+
git clone https://github.com/yourorg/carve
cd carve

# Install all workspace dependencies
pnpm install

# Start the backend API server
make dev-api

# Start the web app
make dev-web

# Build the browser extension (dev mode, Chrome)
make dev-ext-chrome
```

---

## License

- Core platform: AGPL-3.0
- Browser extension: MIT
- Dictionary data: see individual dictionary licenses (JMdict/EDICT: CC BY-SA, etc.)
