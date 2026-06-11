#!/usr/bin/env bash
#
# Seed the local dev stack with a test user + sample cards so every feature has
# data to exercise immediately (review queue, card browser, stats, decks).
# Idempotent-ish: re-running registers a fresh login only if the user is new;
# card creation is idempotent on (user, language, lemma) server-side.
#
# Usage: API_BASE=http://localhost:8080 scripts/seed-dev.sh
set -euo pipefail

API="${API_BASE:-http://localhost:8080}"
EMAIL="${SEED_EMAIL:-dev@carve.app}"
PASS="${SEED_PASS:-devpassword123}"

say() { printf '  %s\n' "$1"; }

# Register (200/201) or, if already exists, log in.
reg=$(curl -fsS -X POST "$API/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\",\"display_name\":\"Dev User\"}" 2>/dev/null || true)
TOKEN=$(printf '%s' "$reg" | python3 -c "import sys,json;print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true)

if [ -z "$TOKEN" ]; then
  login=$(curl -fsS -X POST "$API/v1/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}")
  TOKEN=$(printf '%s' "$login" | python3 -c "import sys,json;print(json.load(sys.stdin)['access_token'])")
  say "logged in existing user $EMAIL"
else
  say "registered $EMAIL"
fi
[ -n "$TOKEN" ] || { echo "✗ could not obtain token"; exit 1; }

card() { # language lemma reading back sentence translation source
  curl -fsS -o /dev/null -X POST "$API/v1/cards" \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -d "$(python3 - "$@" <<'PY'
import json,sys
lang,lemma,reading,back,sentence,trans,src=sys.argv[1:8]
print(json.dumps({"language_code":lang,"lemma":lemma,"reading":reading,
  "back_text":back,"sentence":sentence,"subtitle_translation":trans,
  "source_url":src,"source_timestamp":12.5}))
PY
)" && say "card[$1] $2"
}

# Japanese (full pipeline: reading, sentence, translation, source)
card ja 勉強 べんきょう "study; diligence" "毎日日本語を勉強する" "I study Japanese every day" "https://youtube.com/watch?v=seed1"
card ja 映画 えいが "movie; film" "映画を見ながら勉強する" "I study while watching movies" "https://youtube.com/watch?v=seed2"
card ja 食べる たべる "to eat" "ご飯を食べる" "I eat a meal" "https://youtube.com/watch?v=seed3"
card ja 図書館 としょかん "library" "図書館で本を読む" "I read books at the library" ""
# English (intermediate+, monolingual)
card en ubiquitous "" "present everywhere" "Smartphones are now ubiquitous." "" "https://en.wikipedia.org/wiki/Smartphone"
card en mitigate "" "make less severe" "Measures to mitigate climate change." "" ""
# Spanish
card es gato "" "cat" "El gato negro duerme." "The black cat sleeps." ""

echo "✓ Seed complete. Login: $EMAIL / $PASS"
echo "  Cards span ja/en/es with readings, sentences, translations + source links."
echo "  Open the web app → Review to start; word/sentence audio populates in the"
echo "  background when Google creds are configured."
