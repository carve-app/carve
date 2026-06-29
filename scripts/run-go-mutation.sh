#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly ROOT
readonly API_DIR="$ROOT/services/api"
readonly THRESHOLD="0.65"
readonly REPORT="$API_DIR/report.json"
readonly FUNCTION_MATCH='^(computeDiff|normalize|intervalDays|initialStability|Preview|Handler)$'
readonly SOURCE_FILES=(
  "./internal/fsrs/fsrs.go"
  "./internal/output/transcribe.go"
  "./internal/metrics/metrics.go"
)
readonly TARGETS=(
  "./internal/fsrs/..."
  "./internal/output/..."
  "./internal/metrics/..."
)

restore_interrupted_mutations() {
  local target
  for target in "${SOURCE_FILES[@]}"; do
    if [[ -f "$API_DIR/${target#./}.tmp" ]]; then
      mv "$API_DIR/${target#./}.tmp" "$API_DIR/${target#./}"
    fi
  done
}
trap restore_interrupted_mutations EXIT INT TERM

cd "$API_DIR"
rm -f "$REPORT"
go-mutesting \
  --config .mutation.yml \
  --exec-timeout 30 \
  --match "$FUNCTION_MATCH" \
  "${TARGETS[@]}"

python3 - "$REPORT" "$THRESHOLD" <<'PY'
import json
import sys
from pathlib import Path

report_path = Path(sys.argv[1])
threshold = float(sys.argv[2])
if not report_path.is_file():
    raise SystemExit("go-mutesting did not create report.json")

stats = json.loads(report_path.read_text())["stats"]
killed = int(stats["killedCount"])
escaped = int(stats["escapedCount"])
skipped = int(stats["skippedCount"])
errors = int(stats["errorCount"])
timeouts = int(stats["timeOutCount"])
decided = killed + escaped

if errors or timeouts:
    raise SystemExit(
        f"go-mutesting had {errors} internal errors and {timeouts} timeouts"
    )
if not decided:
    raise SystemExit("go-mutesting produced no decided mutants")

score = killed / decided
print(
    f"Go mutation score: {score:.1%} "
    f"({killed} killed, {escaped} survived, {skipped} skipped)"
)
if score < threshold:
    raise SystemExit(f"Go mutation score is below {threshold:.0%}")
PY
