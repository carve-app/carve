#!/usr/bin/env bash
#
# Full local dev environment for manual testing of ALL Carve features.
#
# Brings up, in one command:
#   - postgres + redis            (Docker; migrations applied)
#   - media service  :8002        (local-disk storage, no MinIO needed)
#   - nlp service    :8001        (FastAPI + the multilingual dictionary)
#   - core api       :8080        (wired to nlp + media; Google creds if present)
#   - web app        :5173        (SvelteKit, points at the api)
#
# Logs stream to dev-logs/<service>.log and a combined tail to the console.
# Ctrl+C tears everything down cleanly (app processes + leaves Docker infra up
# unless --down-on-exit is passed).
#
# Optional Google Cloud (best-on-market TTS + translation): if a service-account
# JSON is found (CARVE_GOOGLE_CREDS env, or ~/Downloads/default-*.json), it is
# wired into api + nlp so audio synthesis and fluent translation work live.
#
# Usage:
#   scripts/dev.sh                 # start everything
#   scripts/dev.sh --seed          # ... and create a test user + sample cards
#   scripts/dev.sh --down-on-exit  # also stop Docker infra on Ctrl+C
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

LOGDIR="$ROOT/dev-logs"
MEDIA_DIR="$ROOT/dev-data/media"
mkdir -p "$LOGDIR" "$MEDIA_DIR"

# ── Config (override via env) ──────────────────────────────────────────────
export DATABASE_URL="${DATABASE_URL:-postgres://carve:carve@localhost:5432/carve?sslmode=disable}"
export JWT_SECRET="${JWT_SECRET:-dev-secret-change-in-production-at-least-32-chars}"
SEED_EMAIL="${SEED_EMAIL:-dev@carve.app}"
SEED_PASS="${SEED_PASS:-devpassword123}"
NLP_PORT=8001
MEDIA_PORT=8002
API_PORT=8080
WEB_PORT=5173

SEED=0
DOWN_ON_EXIT=0
for arg in "$@"; do
  case "$arg" in
    --seed) SEED=1 ;;
    --down-on-exit) DOWN_ON_EXIT=1 ;;
    *) echo "unknown arg: $arg"; exit 1 ;;
  esac
done

# ── Locate optional Google service-account creds ────────────────────────────
GOOGLE_CREDS="${CARVE_GOOGLE_CREDS:-${GOOGLE_APPLICATION_CREDENTIALS:-}}"
if [ -z "$GOOGLE_CREDS" ]; then
  # Best-effort: pick the first SA-looking JSON in ~/Downloads.
  GOOGLE_CREDS="$(find "$HOME/Downloads" -maxdepth 1 -type f -name 'default-*.json' -print 2>/dev/null | head -n 1 || true)"
fi
if [ -n "$GOOGLE_CREDS" ] && [ -f "$GOOGLE_CREDS" ]; then
  export GOOGLE_APPLICATION_CREDENTIALS="$GOOGLE_CREDS"
  echo "✓ Google creds: $GOOGLE_CREDS (TTS + translation enabled)"
else
  echo "ⓘ No Google service-account JSON found — TTS + fluent translation will be"
  echo "  disabled (cards get no audio/translation). Set CARVE_GOOGLE_CREDS to enable."
fi

PIDS=()
cleanup() {
  echo ""
  echo "→ Stopping app services..."
  for pid in "${PIDS[@]:-}"; do kill "$pid" 2>/dev/null || true; done
  wait 2>/dev/null || true
  if [ "$DOWN_ON_EXIT" = "1" ]; then
    echo "→ Stopping Docker infra..."
    docker compose down 2>/dev/null || docker-compose down 2>/dev/null || true
  else
    echo "ⓘ Docker infra (postgres/redis) left running. Stop with: make infra-down"
  fi
  echo "✓ Dev environment stopped."
}
trap cleanup EXIT INT TERM

dc() { docker compose "$@" 2>/dev/null || docker-compose "$@"; }

wait_http() { # url, name, tries
  local url="$1" name="$2" tries="${3:-40}"
  for _ in $(seq 1 "$tries"); do
    if curl -fsS "$url" >/dev/null 2>&1; then echo "  ✓ $name up ($url)"; return 0; fi
    sleep 0.5
  done
  echo "  ✗ $name did not come up at $url — see $LOGDIR/$name.log"; return 1
}

# ── 1. Infra ────────────────────────────────────────────────────────────────
echo "→ [1/6] Starting postgres + redis + Mailpit (Docker)..."
dc up -d postgres redis mailpit
for _ in $(seq 1 40); do
  dc exec -T postgres pg_isready -U carve -q 2>/dev/null && break
  sleep 0.5
done
dc exec -T postgres pg_isready -U carve -q 2>/dev/null || { echo "✗ postgres not ready"; exit 1; }
echo "  ✓ postgres ready"

# ── 2. Migrations ─────────────────────────────────────────────────────────
echo "→ [2/6] Applying migrations..."
if ( cd services/api && go run ./cmd/migrate --dir ./migrations ) >"$LOGDIR/migrate.log" 2>&1; then
  echo "  ✓ migrations applied"
else
  echo "  ✗ migrations failed — see $LOGDIR/migrate.log"
  exit 1
fi

# ── 3. Media service (local-disk storage) ──────────────────────────────────
echo "→ [3/6] Starting media service :$MEDIA_PORT (local disk: $MEDIA_DIR)..."
( cd services/media && \
    STORAGE_BACKEND=local MEDIA_STORAGE_DIR="$MEDIA_DIR" PORT=$MEDIA_PORT \
    go run ./cmd/media ) >"$LOGDIR/media.log" 2>&1 &
PIDS+=($!)
wait_http "http://localhost:$MEDIA_PORT/health" media

# ── 4. NLP service ──────────────────────────────────────────────────────────
echo "→ [4/6] Starting nlp service :$NLP_PORT..."
if [ ! -x services/nlp/.venv/bin/uvicorn ]; then
  echo "  ✗ NLP venv missing. Run 'make setup' first (creates services/nlp/.venv)."; exit 1
fi
if [ ! -f services/nlp/data/dictionary.db ]; then
  echo "  ⚠ dictionary.db missing — run 'make import-all' for full multilingual lookups."
fi
( cd services/nlp && \
    DICT_DB_PATH="$ROOT/services/nlp/data/dictionary.db" \
    GOOGLE_APPLICATION_CREDENTIALS="${GOOGLE_APPLICATION_CREDENTIALS:-}" \
    .venv/bin/uvicorn src.app:app --port $NLP_PORT ) >"$LOGDIR/nlp.log" 2>&1 &
PIDS+=($!)
wait_http "http://localhost:$NLP_PORT/health" nlp

# ── 5. Core API ─────────────────────────────────────────────────────────────
echo "→ [5/6] Starting core api :$API_PORT..."
( cd services/api && \
    DATABASE_URL="$DATABASE_URL" \
    JWT_SECRET="$JWT_SECRET" \
    PORT=$API_PORT \
    NLP_SERVICE_URL="http://localhost:$NLP_PORT" \
    MEDIA_SERVICE_URL="http://localhost:$MEDIA_PORT" \
    GOOGLE_APPLICATION_CREDENTIALS="${GOOGLE_APPLICATION_CREDENTIALS:-}" \
    SMTP_HOST="${SMTP_HOST:-localhost}" \
    SMTP_PORT="${SMTP_PORT:-1025}" \
    SMTP_FROM="${SMTP_FROM:-no-reply@carve.local}" \
    COOKIE_INSECURE=1 \
    go run ./cmd/api ) >"$LOGDIR/api.log" 2>&1 &
PIDS+=($!)
wait_http "http://localhost:$API_PORT/health" api

# ── 6. Web app ──────────────────────────────────────────────────────────────
echo "→ [6/6] Starting web app :$WEB_PORT..."
( VITE_API_BASE="http://localhost:$API_PORT" \
    mise exec -- pnpm --filter @carve/web dev -- --port $WEB_PORT --host >"$LOGDIR/web.log" 2>&1 ) &
PIDS+=($!)
wait_http "http://localhost:$WEB_PORT" web 60 || true  # vite can take a moment

# ── Optional seed ─────────────────────────────────────────────────────────
if [ "$SEED" = "1" ]; then
  echo "→ Seeding test data..."
  API_BASE="http://localhost:$API_PORT" SEED_EMAIL="$SEED_EMAIL" SEED_PASS="$SEED_PASS" \
    bash scripts/seed-dev.sh || echo "  ⚠ seed failed (see output above)"
fi

cat <<EOF

════════════════════════════════════════════════════════════════════
  Carve dev environment is UP. Manual-test surfaces:

  Web app        http://localhost:$WEB_PORT       (register/login, review, cards,
                                          decks, library, stats, output, settings)
  Core API       http://localhost:$API_PORT/health
  NLP service    http://localhost:$NLP_PORT/health   (tokenize/lookup/translate)
  Media service  http://localhost:$MEDIA_PORT/health
  Test email     http://localhost:8025             (Mailpit inbox)
  Postgres       localhost:5432   redis localhost:6379

  Local credentials:
    App login     $SEED_EMAIL / $SEED_PASS $( [ "$SEED" = "1" ] && echo "(seeded this run)" || echo "(run make seed once if missing)" )
    Postgres      $DATABASE_URL
    Redis         no password
    MinIO         minioadmin / minioadmin (only when minio is started)

  Browser extension (Netflix/YouTube mining, popup, annotation):
    built at apps/extension/dist/chrome — load it unpacked in Chrome
    (chrome://extensions → Developer mode → Load unpacked). It defaults to
    http://localhost:$API_PORT.

  Logs:   tail -f dev-logs/*.log
  Stop:   Ctrl+C
════════════════════════════════════════════════════════════════════

→ Streaming combined logs (Ctrl+C to stop everything)...
EOF

tail -n 0 -f "$LOGDIR"/api.log "$LOGDIR"/nlp.log "$LOGDIR"/media.log "$LOGDIR"/web.log &
PIDS+=($!)
wait
