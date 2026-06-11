#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

backup="${1:-}"
if [ -z "$backup" ]; then
  echo "Usage: CONFIRM_RESTORE=1 ./scripts/restore-postgres.sh backups/postgres/carve-postgres-YYYY.sql.gz"
  exit 1
fi

if [ "${CONFIRM_RESTORE:-}" != "1" ]; then
  echo "Refusing to restore without CONFIRM_RESTORE=1."
  echo "Restore into an empty database or after you have taken a fresh backup."
  exit 1
fi

ENV_FILE="${ENV_FILE:-.env}"
RELEASE_FILE="${RELEASE_FILE:-release.env}"

set -a
. "./$ENV_FILE"
[ -f "$RELEASE_FILE" ] && . "./$RELEASE_FILE"
set +a

compose() {
  docker compose --env-file "$ENV_FILE" --env-file "$RELEASE_FILE" -f docker-compose.yml "$@"
}

profile_enabled() {
  key="$1"
  profiles="${COMPOSE_PROFILES:-}"
  case ",$profiles," in
    *,"$key",*) return 0 ;;
    *) return 1 ;;
  esac
}

echo "Restoring $backup..."
if profile_enabled local-db; then
  gzip -dc "$backup" | compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"
else
  gzip -dc "$backup" | docker run --rm -i postgres:16-alpine psql "$DATABASE_URL"
fi

echo "Restore complete."
