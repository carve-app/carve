#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

ENV_FILE="${ENV_FILE:-.env}"
RELEASE_FILE="${RELEASE_FILE:-release.env}"

if [ ! -f "$ENV_FILE" ]; then
  echo "$ENV_FILE is missing. Run ./scripts/bootstrap-env.sh first."
  exit 1
fi

if [ ! -f "$RELEASE_FILE" ]; then
  cp release.env.example "$RELEASE_FILE"
fi

compose() {
  docker compose --env-file "$ENV_FILE" --env-file "$RELEASE_FILE" -f docker-compose.yml "$@"
}

env_value() {
  key="$1"
  awk -F= -v key="$key" '$1 == key { value = substr($0, length($1) + 2) } END { print value }' "$ENV_FILE"
}

require_env() {
  key="$1"
  value="$(env_value "$key")"
  case "$value" in
    ""|*__CHANGE_ME*)
      echo "$key is missing or still contains a placeholder in $ENV_FILE."
      exit 1
      ;;
  esac
}

profile_enabled() {
  key="$1"
  profiles="$(grep -E '^COMPOSE_PROFILES=' "$ENV_FILE" | tail -n1 | cut -d= -f2- | tr -d '"' || true)"
  case ",$profiles," in
    *,"$key",*) return 0 ;;
    *) return 1 ;;
  esac
}

for key in DATABASE_URL JWT_SECRET NLP_INTERNAL_SECRET MEDIA_INTERNAL_TOKEN API_DOMAIN MEDIA_UPLOAD_DOMAIN ACME_EMAIL; do
  require_env "$key"
done

storage_backend="$(env_value STORAGE_BACKEND)"
if [ -z "$storage_backend" ] || [ "$storage_backend" = "r2" ]; then
  for key in R2_ACCOUNT_ID R2_BUCKET R2_PUBLIC_BASE R2_ACCESS_KEY_ID R2_SECRET_ACCESS_KEY; do
    require_env "$key"
  done
fi

if profile_enabled local-db; then
  require_env POSTGRES_PASSWORD
fi

echo "Pulling images..."
compose pull

if profile_enabled local-db; then
  echo "Starting local Postgres..."
  compose up -d postgres

  echo "Waiting for Postgres..."
  i=0
  until compose exec -T postgres pg_isready >/dev/null 2>&1; do
    i=$((i + 1))
    if [ "$i" -gt 30 ]; then
      echo "Postgres did not become healthy in time."
      compose logs postgres
      exit 1
    fi
    sleep 2
  done
fi

echo "Running migrations..."
compose run --rm api-migrate

echo "Starting services..."
compose up -d --remove-orphans

echo "Current service state:"
compose ps

echo "Pruning unused Docker images..."
docker image prune -f >/dev/null 2>&1 || true

echo "Done."
