.PHONY: help setup docker-check \
        infra-up infra-down infra-logs \
        dev dev-full dev-seed seed dev-api dev-nlp dev-web \
        test-nlp test-api test-extension test-video-mining test-promo test-all \
        test-property test-integration test-contract test-e2e \
        test-mutation test-mutation-api test-mutation-nlp test-mutation-ts \
        test-perf test-polish test-canary test-synthetic \
        import-jmdict import-tatoeba import-wordnet import-cedict import-kde4 import-freedict import-all migrate \
        lint build clean record-promo \
        docker-up docker-down docker-logs

PYTHON  := services/nlp/.venv/bin/python3.13
NLP_DIR := services/nlp
API_DIR := services/api
MEDIA_DIR := services/media

# Local dev database URL (matches docker-compose postgres service)
DATABASE_URL ?= postgres://carve:carve@localhost:5432/carve?sslmode=disable
NLP_SERVICE_URL ?= http://localhost:8001
JWT_SECRET ?= dev-secret-change-in-production-at-least-32-chars

help:
	@echo ""
	@echo "Carve development"
	@echo ""
	@echo "First-time setup:"
	@echo "  Install Docker (Docker Desktop, Colima, OrbStack, ...) with Compose v2"
	@echo "  make setup"
	@echo ""
	@echo "Daily workflow:"
	@echo "  make dev-full       — launch ALL features for manual testing (infra+media+nlp+api+web)"
	@echo "  make dev-seed       — dev-full + a seeded test user & sample cards"
	@echo "  make seed           — seed a running stack (dev@carve.app / devpassword123)"
	@echo "  make dev            — legacy: api+nlp+web only (no media service)"
	@echo ""
	@echo "Infrastructure:"
	@echo "  make infra-up       — start postgres + redis + minio"
	@echo "  make infra-down     — stop infrastructure containers"
	@echo "  make infra-logs     — tail infrastructure logs"
	@echo "  make migrate        — apply DB migrations against local postgres"
	@echo ""
	@echo "Data:"
	@echo "  make import-jmdict  — download and import JMdict (already done if db exists)"
	@echo "  make import-tatoeba — import Tatoeba sentence pairs"
	@echo "  make import-cedict  — import CC-CEDICT (Chinese, zh)"
	@echo "  make import-kde4    — import Korean curated dictionary (ko)"
	@echo "  make import-freedict— import FreeDict bilingual dicts (es/de/fr/it/pt)"
	@echo "  make import-all     — import every dictionary into one SQLite db"
	@echo ""
	@echo "Testing:"
	@echo "  make test-nlp       — NLP correctness + API tests"
	@echo "  make test-api       — Go API unit tests"
	@echo "  make test-extension — Chrome extension e2e (Playwright, no backend needed)"
	@echo "  make test-promo     — promo demo flow CI smoke test (no video)"
	@echo "  make test-all       — all tests"
	@echo "  make record-promo   — record marketing video (demo/promo-<date>.webm)"
	@echo ""
	@echo "Other:"
	@echo "  make lint           — Go vet + Python compile check"
	@echo "  make build          — build Go binaries to bin/"
	@echo "  make clean          — remove build artifacts"
	@echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────

setup:
	@echo "→ Checking Docker CLI..."
	@command -v docker  >/dev/null 2>&1 || { echo "✗ docker CLI not found. Install Docker Desktop, Colima, or OrbStack."; exit 1; }
	@docker info >/dev/null 2>&1 || { echo "✗ Docker daemon not reachable. Start your Docker runtime first."; exit 1; }
	@echo "→ Setting up Python venv (requires python3.13)..."
	cd $(NLP_DIR) && python3.13 -m venv .venv && .venv/bin/pip install -r requirements.txt
	@echo "→ Downloading Go modules..."
	cd $(API_DIR) && go mod download
	@echo "→ Installing Node dependencies..."
	pnpm install --frozen-lockfile
	@echo ""
	@echo "✓ Setup complete."
	@echo "  Next: make infra-up   (starts postgres/redis/minio)"
	@echo "  Then: make dev-api    make dev-nlp    make dev-web"

# ── Docker ─────────────────────────────────────────────────────────────────────

# Verify a Docker daemon is reachable, whatever the runtime (Docker Desktop,
# Colima, OrbStack, ...). Targets that need containers depend on this.
docker-check:
	@command -v docker >/dev/null 2>&1 || { \
		echo "✗ docker CLI not found. Install Docker Desktop, Colima, or OrbStack."; exit 1; }
	@docker info >/dev/null 2>&1 || { \
		echo "✗ Docker daemon not reachable. Start your Docker runtime first."; exit 1; }

# ── Infrastructure ────────────────────────────────────────────────────────────

infra-up: docker-check
	@echo "→ Starting postgres, redis, minio, mailpit..."
	docker compose up -d postgres redis minio mailpit
	@echo "→ Waiting for postgres to be ready..."
	@for i in $$(seq 1 20); do \
		docker compose exec -T postgres pg_isready -U carve -q && break; \
		echo "  waiting... ($$i/20)"; sleep 2; \
	done
	@docker compose exec -T postgres pg_isready -U carve -q || \
		{ echo "✗ Postgres did not become ready"; exit 1; }
	@echo "→ Running migrations..."
	@$(MAKE) migrate
	@echo ""
	@echo "✓ Infrastructure ready:"
	@echo "  postgres  → localhost:5432"
	@echo "  redis     → localhost:6379"
	@echo "  minio     → localhost:9000  (console: localhost:9001)"
	@echo "  mailpit   → localhost:8025  (SMTP: localhost:1025)"
	@echo ""
	@echo "Run 'make dev' to start all services."

infra-down:
	docker compose down

infra-logs:
	docker compose logs -f postgres redis minio

# ── Development servers ───────────────────────────────────────────────────────

# Full dev environment for manual testing of ALL features: infra + media + nlp
# + api + web, correctly wired (local-disk media, Google creds if present),
# with streaming logs and clean shutdown. This is the recommended entrypoint.
dev-full: docker-check
	@bash scripts/dev.sh

# Same, plus a seeded test user + sample cards (dev@carve.app / devpassword123).
dev-seed: docker-check
	@bash scripts/dev.sh --seed

# Seed an already-running stack with test data.
seed:
	@API_BASE="http://localhost:8080" bash scripts/seed-dev.sh

# Legacy: api+nlp+web only (no media service). Prefer `make dev-full`.
dev: infra-up
	@echo "→ Starting api, nlp, web (Ctrl+C stops all)..."
	@trap 'kill 0' SIGINT; \
	  $(MAKE) dev-api & \
	  $(MAKE) dev-nlp & \
	  $(MAKE) dev-web & \
	  wait

dev-api:
	cd $(API_DIR) && \
	  DATABASE_URL="$(DATABASE_URL)" \
	  NLP_SERVICE_URL="$(NLP_SERVICE_URL)" \
	  JWT_SECRET="$(JWT_SECRET)" \
	  PORT=8080 \
	  go run ./cmd/api

dev-nlp:
	cd $(NLP_DIR) && .venv/bin/uvicorn carve_nlp.app:app --reload --port 8001

dev-web:
	mise exec -- pnpm --filter @carve/web dev

# ── Data import ───────────────────────────────────────────────────────────────

import-jmdict:
	@mkdir -p $(NLP_DIR)/data
	@if [ ! -f $(NLP_DIR)/data/JMdict_e ]; then \
		echo "→ Downloading JMdict..."; \
		curl -sL "http://ftp.edrdg.org/pub/Nihongo/JMdict_e.gz" -o $(NLP_DIR)/data/JMdict_e.gz; \
		gzip -d $(NLP_DIR)/data/JMdict_e.gz; \
	fi
	cd $(NLP_DIR) && $(PYTHON) scripts/import_jmdict.py
	@echo "✓ JMdict imported"

import-tatoeba: $(NLP_DIR)/data/dictionary.db
	cd $(NLP_DIR) && $(PYTHON) scripts/import_tatoeba.py --limit 50000
	@echo "✓ Tatoeba sentences imported"

import-wordnet: $(NLP_DIR)/data/dictionary.db
	cd $(NLP_DIR) && $(PYTHON) scripts/import_wordnet.py
	@echo "✓ WordNet English dictionary imported"

import-cedict: $(NLP_DIR)/data/dictionary.db
	cd $(NLP_DIR) && $(PYTHON) scripts/import_cedict_sqlite.py
	@echo "✓ CC-CEDICT (zh) imported"

import-kde4: $(NLP_DIR)/data/dictionary.db
	cd $(NLP_DIR) && $(PYTHON) scripts/import_kde4_sqlite.py
	@echo "✓ Korean dictionary (ko) imported"

import-freedict: $(NLP_DIR)/data/dictionary.db
	cd $(NLP_DIR) && $(PYTHON) scripts/import_freedict.py
	@echo "✓ FreeDict (es/de/fr/it/pt) imported"

# Build the full multilingual dictionary in one shot.
import-all: import-jmdict import-wordnet import-cedict import-kde4 import-freedict
	@echo "✓ All dictionaries imported"

$(NLP_DIR)/data/dictionary.db:
	@$(MAKE) import-jmdict

# ── Tests ─────────────────────────────────────────────────────────────────────

test-nlp: $(NLP_DIR)/data/dictionary.db
	cd $(NLP_DIR) && .venv/bin/python3.13 -m pytest tests/ -v --tb=short
	@echo "✓ All NLP tests passed"

test-api:
	cd $(API_DIR) && go test ./... -v -race

test-extension:
	@echo "→ Building extension..."
	mise exec -- pnpm --filter @carve/extension build
	@echo "→ Running extension e2e test..."
	cd e2e && node extension.test.js
	@echo "✓ Extension e2e test passed"

# Real end-to-end video sentence-mining test: drives the actual extension in a
# real Chromium against the real Go api + media binaries and a real Postgres,
# then asserts the mined card carries a real screenshot, exact-sentence audio,
# sentence, translation, and cue timing. Self-contained (boots its own pg
# container + services). Skips gracefully if docker/artifacts are unavailable.
test-video-mining: build
	@echo "→ Building media binary + chrome extension..."
	cd $(MEDIA_DIR) && go build -o ../../bin/media ./cmd/media
	mise exec -- pnpm --filter @carve/extension build:chrome
	@echo "→ Generating the audio/video fixture (vp8 + opus)..."
	@mkdir -p e2e/fixtures/media
	@test -f e2e/fixtures/media/sample.webm || ffmpeg -hide_banner -loglevel error -y \
		-f lavfi -i "color=c=0x1e88e5:s=480x270:d=6,format=yuv420p" \
		-f lavfi -i "sine=frequency=440:duration=6" \
		-c:v libvpx -b:v 400k -c:a libopus -b:a 96k -shortest \
		e2e/fixtures/media/sample.webm
	@echo "→ Running real video-mining e2e..."
	cd e2e && node video-mining.test.js
	@echo "✓ Video sentence-mining e2e passed"

test-promo:
	@echo "→ Running promo demo flow (CI mode — no video saved)..."
	cd e2e && node ../demo/record-promo.js --ci
	@echo "✓ Promo demo flow passed"

record-promo:
	@echo "→ Recording promotional demo video..."
	@echo "   Output: demo/promo-<date>.webm"
	cd e2e && node ../demo/record-promo.js
	@echo "✓ Video saved in demo/"

test-all: test-nlp test-api test-extension test-promo
	@echo "✓ All tests passed — Phase 0 gate cleared"

# ── Per-layer test targets (docs/12-testing-strategy.md) ─────────────────────

# L1 — property-based
test-property:
	@echo "→ L1 property-based tests..."
	cd $(API_DIR) && go test ./internal/fsrs/... ./internal/importer/... ./internal/output/... -run "Property" -count=1
	cd $(NLP_DIR) && .venv/bin/python -m pytest tests/test_property_en.py -q
	mise exec -- pnpm --filter @carve/extension exec vitest run src/content/popup/__tests__/property.test.ts

# L3 — testcontainers integration
test-integration: docker-check
	@echo "→ L3 testcontainers integration tests..."
	cd $(API_DIR) && TESTCONTAINERS_RYUK_DISABLED=true \
		go test ./internal/importer/... -run TestImportAnki_ -count=1 -timeout 10m

# L4 — OpenAPI contract + schemathesis fuzz
test-contract:
	@command -v schemathesis >/dev/null 2>&1 || { echo "✗ pip install schemathesis==4.4.4"; exit 1; }
	@[ -n "$$API_BASE" ] || { echo "✗ API_BASE must point at a running isolated API"; exit 1; }
	@[ -n "$$ACCESS_TOKEN" ] || { echo "✗ ACCESS_TOKEN is required for authenticated contract coverage"; exit 1; }
	schemathesis run docs/openapi.yaml --url "$$API_BASE" --max-examples 25 \
		--request-timeout 5 \
		--checks not_a_server_error,status_code_conformance,content_type_conformance \
		--header "Authorization: Bearer $$ACCESS_TOKEN"

# L5/L6/L7/L8/L9 — Playwright matrix (web + extension + visual + a11y + offline)
test-e2e:
	@echo "→ L5–L9 Playwright matrix..."
	mise exec -- pnpm --filter e2e test:web

# L2 — mutation testing per stack
test-mutation: test-mutation-api test-mutation-nlp test-mutation-ts

test-mutation-api:
	@command -v go-mutesting >/dev/null 2>&1 || { echo "✗ go install github.com/avito-tech/go-mutesting/cmd/go-mutesting@v0.0.0-20251226130216-48d0401f00fb"; exit 1; }
	./scripts/run-go-mutation.sh

test-mutation-nlp:
	@command -v mutmut >/dev/null 2>&1 || { echo "✗ pip install mutmut==3.6.0"; exit 1; }
	cd $(NLP_DIR) && mutmut run

test-mutation-ts:
	@echo "→ L2 Stryker mutation testing (TS)..."
	mise exec -- pnpm --filter @carve/extension exec stryker run
	mise exec -- pnpm --filter @carve/web exec stryker run

# L13 — perf budget
test-perf:
	@command -v k6 >/dev/null 2>&1 || { echo "✗ brew install k6"; exit 1; }
	k6 run tests/perf/load.js
	@echo "→ Lighthouse CI..."
	mise exec -- pnpm exec lhci autorun

# L14 — LLM-as-judge polish
test-polish:
	@[ -n "$$ANTHROPIC_API_KEY" ] || { echo "✗ ANTHROPIC_API_KEY not set"; exit 1; }
	node scripts/polish-review.mjs

# L11 — streaming canary (hourly cron)
test-canary:
	@echo "→ L11 hourly streaming canary..."
	mise exec -- pnpm --filter e2e exec playwright test tests/extension-streaming.spec.ts

# L15 — production synthetic (every minute via cron)
test-synthetic:
	@[ -n "$$API_BASE" ] || { echo "✗ API_BASE not set"; exit 1; }
	node scripts/synthetic.mjs

# ── Database ──────────────────────────────────────────────────────────────────

migrate:
	cd $(API_DIR) && DATABASE_URL="$(DATABASE_URL)" go run ./cmd/migrate --dir ./migrations

# ── Lint ──────────────────────────────────────────────────────────────────────

lint:
	cd $(API_DIR) && go vet ./...
	cd $(NLP_DIR) && .venv/bin/python3.13 -m py_compile src/*.py
	@echo "✓ Lint passed"

# ── Build ─────────────────────────────────────────────────────────────────────

build:
	cd $(API_DIR) && go build -o ../../bin/api ./cmd/api && go build -o ../../bin/migrate ./cmd/migrate
	@echo "✓ Go binaries built to bin/"

# ── Docker (full stack, for CI parity) ───────────────────────────────────────

docker-up: docker-check
	docker compose up -d --build

docker-down:
	docker compose down -v

docker-logs:
	docker compose logs -f

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/
	cd $(API_DIR) && go clean
