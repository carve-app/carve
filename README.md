# Carve

> Cut through the noise. Carve fluency from real content.

Carve is a science-grounded language-learning platform for serious immersion
learners — a single, open, cross-platform replacement for the Anki + Yomitan +
Migaku toolchain: accurate parsing, transparent SRS, and sentence mining from
real video and web content.

**Current build state:** see [docs/STATUS.md](docs/STATUS.md) for what actually
works end-to-end today (verified), what's partial, and the known gaps. The
numbered `docs/NN-*.md` files are the original design specs and remain useful as
architecture/reference, but STATUS.md is the source of truth for the live state.

---

## What works today (verified end-to-end)

- **Sentence mining from video** — press `m` on a subtitle to create a card with
  a DRM-safe screenshot, exact-sentence audio, the sentence, a fluent
  translation, and the cue's source timing. Works across YouTube/Netflix/
  Disney+/Prime/Crunchyroll/Viki, including on real SPA navigation.
- **In-page word coloring + click-to-look-up** — tokenized, status-colored
  (known/learning/unknown) annotation with a popup showing definition, reading,
  pitch accent, frequency band, a dictionary image, word audio, and an AI
  contextual explanation.
- **9 languages end-to-end** — Japanese, English (monolingual, intermediate+),
  Chinese, Korean, Spanish, German, French, Italian, Portuguese: tokenize →
  dictionary lookup → mine. (Vietnamese tokenizes; no dictionary yet.)
- **Best-on-market MT + TTS** — fluent translation via Google Cloud Translation
  v3 (Translation LLM) and word/sentence audio via Google Cloud Text-to-Speech,
  both via a service account. No degraded fallbacks.
- **FSRS-6 SRS review** (web) with recognition/production card types, sentence +
  word audio, images, and translation; offline review event queue.
- **Grammar tracking**, **Anki `.apkg` + CSV export**, **Anki/Yomitan/JPDB/Migaku
  import**, **immersion time tracking**, **dictionary/comprehension overlay**.
- **Sessions don't expire mid-use** — 4h access tokens with transparent rotating
  refresh on both web and extension.

Web app, Go API, Python NLP, and the media service all run locally with one
command (below). The extension is loaded unpacked.

---

## Stack

| Component | Tech | Path |
|---|---|---|
| Core API | Go 1.26 (chi, pgx/Postgres) | `services/api` |
| NLP service | Python 3.13 (FastAPI, SudachiPy, SQLite dicts) | `services/nlp` |
| Media service | Go (local-disk dev / Cloudflare R2 prod) | `services/media` |
| Web app | SvelteKit | `apps/web` |
| Browser extension | TypeScript MV3 (Chrome/Firefox/Safari) | `apps/extension` |
| Infra | Terraform (AWS ECS, RDS, ElastiCache, R2, SES) | `infra/terraform` |

---

## Quick start (development)

Prerequisites: Docker, Go 1.26+, Python 3.13, Node 22+ (pnpm), `ffmpeg` (for the
video-mining e2e). Optional: a Google Cloud service-account JSON to enable live
TTS + translation.

```bash
# One-time: venv, Go modules, node deps, and the multilingual dictionary.
make setup
make import-all        # builds services/nlp/data/dictionary.db (JA/EN/ZH/KO/ES/DE/FR/IT/PT)

# Launch the FULL stack for manual testing of every feature:
#   postgres + redis (Docker) · media · nlp · api · web, correctly wired,
#   with streaming logs and clean Ctrl+C teardown.
make dev-seed          # also seeds a test user: dev@carve.app / devpassword123

# To enable live TTS + translation, point this at your service-account JSON:
#   CARVE_GOOGLE_CREDS=/path/to/sa.json make dev-seed
```

Surfaces once up: web `http://localhost:5173`, API `:8080`, NLP `:8001`,
media `:8002`. Load the extension from `apps/extension/dist/chrome` (build it
with `npm --prefix apps/extension run build:chrome`) via
`chrome://extensions → Developer mode → Load unpacked`; it defaults to the
local API.

```bash
# Tests
make test-api          # Go: vet + race tests
make test-nlp          # Python: pytest
make test-extension    # built extension e2e (Playwright, mock backend)
make test-video-mining # real full-stack video-mining e2e (needs Docker + ffmpeg)
cd apps/web && npm run check   # svelte-check
```

---

## License

- Core platform: AGPL-3.0
- Browser extension: MIT
- Dictionary data: per source (JMdict/EDICT CC BY-SA, CC-CEDICT, WordNet,
  FreeDict, Tatoeba — see each importer in `services/nlp/scripts/`).
