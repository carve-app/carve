#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

ENV_FILE="${ENV_FILE:-.env}"
RELEASE_FILE="${RELEASE_FILE:-release.env}"

if [ ! -f "$ENV_FILE" ]; then
  echo "$ENV_FILE is missing."
  exit 1
fi

set -a
. "./$ENV_FILE"
[ -f "$RELEASE_FILE" ] && . "./$RELEASE_FILE"
set +a

BACKUP_DIR="${BACKUP_DIR:-./backups/postgres}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"
mkdir -p "$BACKUP_DIR"

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

ts="$(date -u +%Y%m%dT%H%M%SZ)"
out="$BACKUP_DIR/carve-postgres-$ts.sql.gz"

echo "Writing $out..."
if profile_enabled local-db; then
  compose exec -T postgres pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" | gzip -9 > "$out"
else
  docker run --rm postgres:16-alpine pg_dump "$DATABASE_URL" | gzip -9 > "$out"
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "$out" > "$out.sha256"
else
  shasum -a 256 "$out" > "$out.sha256"
fi

if [ -n "${R2_BACKUP_BUCKET:-}" ]; then
  backup_abs="$(cd "$BACKUP_DIR" && pwd)"
  backup_name="$(basename "$out")"
  endpoint="https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com"
  key_id="${R2_BACKUP_ACCESS_KEY_ID:-${R2_ACCESS_KEY_ID:-}}"
  secret_key="${R2_BACKUP_SECRET_ACCESS_KEY:-${R2_SECRET_ACCESS_KEY:-}}"

  if [ -z "$key_id" ] || [ -z "$secret_key" ] || [ -z "${R2_ACCOUNT_ID:-}" ]; then
    echo "R2_BACKUP_BUCKET is set, but R2 backup credentials/account are missing."
    exit 1
  fi

  echo "Uploading $backup_name to R2 bucket $R2_BACKUP_BUCKET..."
  docker run --rm \
    -e AWS_ACCESS_KEY_ID="$key_id" \
    -e AWS_SECRET_ACCESS_KEY="$secret_key" \
    -e AWS_DEFAULT_REGION=auto \
    -v "$backup_abs:/backup:ro" \
    amazon/aws-cli:2.17.40 \
    s3 cp "/backup/$backup_name" "s3://$R2_BACKUP_BUCKET/${R2_BACKUP_PREFIX:-phase1/postgres}/$backup_name" \
    --endpoint-url "$endpoint"

  docker run --rm \
    -e AWS_ACCESS_KEY_ID="$key_id" \
    -e AWS_SECRET_ACCESS_KEY="$secret_key" \
    -e AWS_DEFAULT_REGION=auto \
    -v "$backup_abs:/backup:ro" \
    amazon/aws-cli:2.17.40 \
    s3 cp "/backup/$backup_name.sha256" "s3://$R2_BACKUP_BUCKET/${R2_BACKUP_PREFIX:-phase1/postgres}/$backup_name.sha256" \
    --endpoint-url "$endpoint"
fi

find "$BACKUP_DIR" -type f -name 'carve-postgres-*.sql.gz*' -mtime +"$BACKUP_RETENTION_DAYS" -delete

echo "Backup complete."
