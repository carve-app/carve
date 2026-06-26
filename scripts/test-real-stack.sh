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

if command -v pnpm >/dev/null 2>&1; then
  PNPM=(pnpm)
else
  PNPM=(mise exec -- pnpm)
fi

cleanup() {
  docker compose down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

docker compose up -d --build postgres redis mailpit minio nlp media
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

E2E_USE_REAL=1 \
API_BASE="http://127.0.0.1:${API_PORT}" \
MAILPIT_BASE="http://127.0.0.1:${MAILPIT_HTTP_PORT}" \
WEB_BASE_URL=http://127.0.0.1:5173 \
"${PNPM[@]}" --dir e2e exec playwright test tests/real-stack-core.spec.ts --project=web-chromium
