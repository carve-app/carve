#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

if [ -f .env ]; then
  echo ".env already exists; refusing to overwrite it."
  exit 1
fi

if [ ! -f env.example ]; then
  echo "env.example is missing."
  exit 1
fi

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required to generate secrets."
  exit 1
fi

cp env.example .env

secret() {
  openssl rand -hex 32
}

replace() {
  needle="$1"
  value="$2"
  tmp="$(mktemp)"
  sed "s|$needle|$value|g" .env > "$tmp"
  mv "$tmp" .env
}

postgres_password="$(secret)"
replace "__CHANGE_ME_POSTGRES_PASSWORD__" "$postgres_password"
replace "__CHANGE_ME_JWT_SECRET__" "$(secret)"
replace "__CHANGE_ME_NLP_INTERNAL_SECRET__" "$(secret)"
replace "__CHANGE_ME_MEDIA_INTERNAL_TOKEN__" "$(secret)"

if [ ! -f release.env ]; then
  cp release.env.example release.env
fi

chmod 600 .env release.env

cat <<'MSG'
Created .env and release.env.

Next:
  1. Set DATABASE_URL from: terraform output -raw database_url
  2. Edit .env R2 settings and optional external keys.
  3. Confirm Terraform-managed DNS for API_DOMAIN and MEDIA_UPLOAD_DOMAIN.
  4. Run ./scripts/deploy.sh.
MSG
