.PHONY: help setup \
        colima-start infra-up infra-down infra-logs \
        dev-api dev-nlp \
        test-nlp test-api test-all \
        import-jmdict import-tatoeba migrate \
        lint build clean \
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
	@echo "Carve development — no Docker Desktop required"
	@echo ""
	@echo "First-time setup:"
	@echo "  brew install colima docker docker-compose"
	@echo "  make setup"
	@echo ""
	@echo "Daily workflow (3 terminals):"
	@echo "  make infra-up       — start Colima + postgres + redis + minio, apply migrations"
	@echo "  make dev-api        — terminal 1: Go API on :8080"
	@echo "  make dev-nlp        — terminal 2: Python NLP on :8001"
	@echo "  make dev-web        — terminal 3: SvelteKit on :5173"
	@echo ""
	@echo "Infrastructure:"
	@echo "  make infra-up       — start postgres + redis + minio (via Colima)"
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
	@echo "  make test-all       — all tests"
	@echo ""
	@echo "Other:"
	@echo "  make lint           — Go vet + Python compile check"
	@echo "  make build          — build Go binaries to bin/"
	@echo "  make clean          — remove build artifacts"
	@echo ""

# ── Setup ─────────────────────────────────────────────────────────────────────

setup:
	@echo "→ Checking Colima + Docker CLI..."
	@command -v colima >/dev/null 2>&1 || { echo "✗ colima not found. Run: brew install colima docker docker-compose"; exit 1; }
	@command -v docker  >/dev/null 2>&1 || { echo "✗ docker CLI not found. Run: brew install docker"; exit 1; }
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

# ── Colima (Docker Desktop replacement) ───────────────────────────────────────

colima-start:
	@command -v colima >/dev/null 2>&1 || { \
		echo "✗ colima not found. Run: brew install colima docker docker-compose"; exit 1; }
	@if colima status 2>/dev/null | grep -q "Running"; then \
		echo "✓ Colima already running"; \
	else \
		echo "→ Starting Colima..."; \
		colima start --cpu 2 --memory 4; \
		echo "✓ Colima started"; \
	fi

# ── Infrastructure ────────────────────────────────────────────────────────────

infra-up: colima-start
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
	@echo "Start services in separate terminals:"
	@echo "  make dev-api    make dev-nlp    make dev-web"

infra-down:
	docker-compose down

infra-logs:
	docker-compose logs -f postgres redis minio

# ── Development servers ───────────────────────────────────────────────────────

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

test-all: test-nlp test-api
	@echo "✓ All tests passed — Phase 0 gate cleared"

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

docker-up: colima-start
	docker-compose up -d --build

docker-down:
	docker-compose down -v

docker-logs:
	docker-compose logs -f

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/
	cd $(API_DIR) && go clean
