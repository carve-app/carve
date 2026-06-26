#!/usr/bin/env bash
# Boots the actual Carve services against isolated Docker volumes, runs the
# real-service Playwright proof, and tears everything down even on failure.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-carve-proof}"
export POSTGRES_PORT="${POSTGRES_PORT:-15432}"
export REDIS_PORT="${REDIS_PORT:-16379}"
export MAILPIT_SMTP_PORT="${MAILPIT_SMTP_PORT:-11025}"
export MAILPIT_HTTP_PORT="${MAILPIT_HTTP_PORT:-18025}"
export MINIO_PORT="${MINIO_PORT:-19000}"
export MINIO_CONSOLE_PORT="${MINIO_CONSOLE_PORT:-19001}"
export API_PORT="${API_PORT:-18080}"
export NLP_PORT="${NLP_PORT:-18001}"
export MEDIA_PORT="${MEDIA_PORT:-18002}"
export MEDIA_PUBLIC_BASE="${MEDIA_PUBLIC_BASE:-http://127.0.0.1:${MEDIA_PORT}}"

if command -v pnpm >/dev/null 2>&1; then
  PNPM=(pnpm)
  PNPM_COMMAND=pnpm
else
  PNPM=(mise exec -- pnpm)
  PNPM_COMMAND="mise exec -- pnpm"
fi

cleanup() {
  local status=$?
  if [ "$status" -ne 0 ]; then
    docker compose logs --tail=200 api migrate nlp media mailpit || true
  fi
  docker compose down -v --remove-orphans >/dev/null 2>&1 || true
  return "$status"
}
trap cleanup EXIT INT TERM

# Build every source-backed service explicitly. `docker compose run migrate`
# and `up api` do not rebuild by default, which can otherwise make this proof
# execute stale binaries and silently omit newly-added migrations.
docker compose build api migrate nlp media
docker compose up -d postgres redis mailpit minio nlp media
docker compose run --rm migrate
docker compose up -d api

for attempt in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:${API_PORT}/health/ready" >/dev/null && \
     curl -fsS "http://127.0.0.1:${MAILPIT_HTTP_PORT}/api/v1/info" >/dev/null; then
    break
  fi
  if [ "$attempt" -eq 60 ]; then
    docker compose ps
    docker compose logs --tail=200 api nlp media mailpit
    exit 1
  fi
  sleep 2
done

PLAYWRIGHT_ARGS=(tests/real-stack-core.spec.ts --project=web-chromium)
if [ -n "${REAL_STACK_GREP:-}" ]; then
  PLAYWRIGHT_ARGS+=(--grep "$REAL_STACK_GREP")
fi

E2E_USE_REAL=1 \
API_BASE="http://127.0.0.1:${API_PORT}" \
MAILPIT_BASE="http://127.0.0.1:${MAILPIT_HTTP_PORT}" \
WEB_BASE_URL=http://127.0.0.1:5173 \
PNPM_COMMAND="$PNPM_COMMAND" \
"${PNPM[@]}" --dir e2e exec playwright test "${PLAYWRIGHT_ARGS[@]}"
