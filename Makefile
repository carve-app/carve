.PHONY: help setup docker-check \
        infra-up infra-down infra-logs \
        dev dev-api dev-nlp dev-web \
        test-nlp test-api test-extension test-promo test-all \
        test-property test-integration test-contract test-e2e \
        test-mutation test-mutation-api test-mutation-nlp test-mutation-ts \
        test-perf test-polish test-canary test-synthetic \
        import-jmdict import-tatoeba migrate \
        lint build clean record-promo \
        docker-up docker-down docker-logs

PYTHON  := services/nlp/.venv/bin/python3.13
NLP_DIR := services/nlp
API_DIR := services/api

# Local dev database URL (matches docker-compose postgres service)
DATABASE_URL ?= postgres://carve:carve@localhost:5432/carve?sslmode=disable
NLP_SERVICE_URL ?= http://localhost:8001
JWT_SECRET ?= dev-secret-change-in-production-at-least-32-chars

help:
	@echo ""
	@echo "Carve development"
	@echo ""
	@echo "First-time setup:"
	@echo "  Install Docker (Docker Desktop, Colima, OrbStack, ...) + docker-compose"
	@echo "  make setup"
	@echo ""
	@echo "Daily workflow:"
	@echo "  make dev            — start everything (infra + api + nlp + web), Ctrl+C stops all"
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
	pnpm install
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
	@echo "→ Starting postgres, redis, minio..."
	docker-compose up -d postgres redis minio
	@echo "→ Waiting for postgres to be ready..."
	@for i in $$(seq 1 20); do \
		docker-compose exec -T postgres pg_isready -U carve -q && break; \
		echo "  waiting... ($$i/20)"; sleep 2; \
	done
	@docker-compose exec -T postgres pg_isready -U carve -q || \
		{ echo "✗ Postgres did not become ready"; exit 1; }
	@echo "→ Running migrations..."
	@$(MAKE) migrate
	@echo ""
	@echo "✓ Infrastructure ready:"
	@echo "  postgres  → localhost:5432"
	@echo "  redis     → localhost:6379"
	@echo "  minio     → localhost:9000  (console: localhost:9001)"
	@echo ""
	@echo "Run 'make dev' to start all services."

infra-down:
	docker-compose down

infra-logs:
	docker-compose logs -f postgres redis minio

# ── Development servers ───────────────────────────────────────────────────────

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
	cd $(NLP_DIR) && .venv/bin/uvicorn src.app:app --reload --port 8001

dev-web:
	cd apps/web && npm run dev

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
	cd apps/extension && npm run build
	@echo "→ Running extension e2e test..."
	cd e2e && node extension.test.js
	@echo "✓ Extension e2e test passed"

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
	cd apps/extension && npx vitest run src/content/popup/__tests__/property.test.ts

# L3 — testcontainers integration
test-integration: docker-check
	@echo "→ L3 testcontainers integration tests..."
	cd $(API_DIR) && TESTCONTAINERS_RYUK_DISABLED=true \
		go test ./internal/importer/... -run TestImportAnki_ -count=1 -timeout 10m

# L4 — OpenAPI contract + schemathesis fuzz
test-contract:
	@command -v schemathesis >/dev/null 2>&1 || { echo "✗ pip install schemathesis"; exit 1; }
	@echo "→ L4 schemathesis fuzz..."
	# Spin up the API on FUZZ_PORT against a transient DB, then run schemathesis.
	FUZZ_PORT=8090 schemathesis run --base-url http://127.0.0.1:8090 docs/openapi.yaml

# L5/L6/L7/L8/L9 — Playwright matrix (web + extension + visual + a11y + offline)
test-e2e:
	@echo "→ L5–L9 Playwright matrix..."
	cd e2e && npm run test:web

# L2 — mutation testing per stack
test-mutation: test-mutation-api test-mutation-nlp test-mutation-ts

test-mutation-api:
	@command -v go-mutesting >/dev/null 2>&1 || { echo "✗ go install github.com/avito-tech/go-mutesting/cmd/go-mutesting@latest"; exit 1; }
	cd $(API_DIR) && go-mutesting --config .mutation.txt --threshold 0.65 \
		./internal/fsrs/... ./internal/importer/... ./internal/output/... ./internal/metrics/...

test-mutation-nlp:
	@command -v mutmut >/dev/null 2>&1 || { echo "✗ pip install mutmut"; exit 1; }
	cd $(NLP_DIR) && mutmut run --paths-to-mutate=src/ \
		--tests-dir=tests/ --runner='.venv/bin/python -m pytest -x --tb=no -q'

test-mutation-ts:
	@echo "→ L2 Stryker mutation testing (TS)..."
	cd apps/extension && npx stryker run
	cd apps/web && npx stryker run

# L13 — perf budget
test-perf:
	@command -v k6 >/dev/null 2>&1 || { echo "✗ brew install k6"; exit 1; }
	k6 run tests/perf/load.js
	@echo "→ Lighthouse CI..."
	@command -v lhci >/dev/null 2>&1 || { echo "✗ npm i -g @lhci/cli"; exit 1; }
	lhci autorun

# L14 — LLM-as-judge polish
test-polish:
	@[ -n "$$ANTHROPIC_API_KEY" ] || { echo "✗ ANTHROPIC_API_KEY not set"; exit 1; }
	node scripts/polish-review.mjs

# L11 — streaming canary (hourly cron)
test-canary:
	@echo "→ L11 hourly streaming canary..."
	cd e2e && npx playwright test tests/extension-streaming.spec.ts

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
	docker-compose up -d --build

docker-down:
	docker-compose down -v

docker-logs:
	docker-compose logs -f

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/
	cd $(API_DIR) && go clean
