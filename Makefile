.PHONY: help dev-api dev-nlp test-nlp test-api test-all \
        import-jmdict import-tatoeba migrate lint build clean

PYTHON := services/nlp/.venv/bin/python3.13
NLP_DIR := services/nlp
API_DIR  := services/api

help:
	@echo "Carve development commands:"
	@echo "  make setup          — Install all dependencies (first-time setup)"
	@echo "  make dev-api        — Run the Go API server (requires Postgres)"
	@echo "  make dev-nlp        — Run the Python NLP service"
	@echo "  make test-nlp       — Run NLP correctness + API tests"
	@echo "  make test-api       — Run Go API unit tests"
	@echo "  make test-all       — Run all tests"
	@echo "  make import-jmdict  — Download and import JMdict"
	@echo "  make import-tatoeba — Import Tatoeba sentence pairs"
	@echo "  make migrate        — Apply database migrations"
	@echo "  make lint           — Run linters (Go vet, Python)"
	@echo "  make build          — Build all services"
	@echo "  make docker-up      — Start all services via Docker Compose"
	@echo "  make docker-down    — Stop and remove containers"

# ── Setup ─────────────────────────────────────────────────────────────────────

setup:
	@echo "→ Setting up Python venv..."
	cd $(NLP_DIR) && python3.13 -m venv .venv && .venv/bin/pip install -r requirements.txt
	@echo "→ Setting up Go modules..."
	cd $(API_DIR) && go mod download
	@echo "→ Installing pnpm dependencies..."
	pnpm install
	@echo "✓ Setup complete. Run 'make import-jmdict' to populate the dictionary."

# ── Development servers ───────────────────────────────────────────────────────

dev-api:
	cd $(API_DIR) && go run ./cmd/api

dev-nlp:
	cd $(NLP_DIR) && .venv/bin/uvicorn src.app:app --reload --port 8001

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
	cd $(API_DIR) && go run ./cmd/migrate --dir ./migrations

# ── Lint ──────────────────────────────────────────────────────────────────────

lint:
	cd $(API_DIR) && go vet ./...
	cd $(NLP_DIR) && .venv/bin/python3.13 -m py_compile src/*.py
	@echo "✓ Lint passed"

# ── Build ─────────────────────────────────────────────────────────────────────

build:
	cd $(API_DIR) && go build -o ../../bin/api ./cmd/api && go build -o ../../bin/migrate ./cmd/migrate
	@echo "✓ Go binaries built to bin/"

# ── Docker ────────────────────────────────────────────────────────────────────

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down -v

docker-logs:
	docker compose logs -f

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/
	cd $(API_DIR) && go clean
