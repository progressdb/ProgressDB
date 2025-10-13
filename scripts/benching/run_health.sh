#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
cd "$ROOT_DIR"

# Simple health endpoint benchmark using k6. Assumes the ProgressDB server is
# already running. Adjust TARGET or RATE env vars as needed.

if ! command -v k6 >/dev/null 2>&1; then
  echo "k6 is required but not installed" >&2
  exit 1
fi

TARGET=${TARGET:-http://127.0.0.1:8080/healthz}
K6_SCRIPT=${K6_SCRIPT:-service/tests/benching/k6/health.js}

ARTIFACT_ROOT=${TEST_ARTIFACTS_ROOT:-${PROGRESSDB_ARTIFACT_ROOT:-"$ROOT_DIR/tests/artifacts"}}
mkdir -p "$ARTIFACT_ROOT"
ARTIFACT_ROOT="$(cd "$ARTIFACT_ROOT" && pwd)"
export PROGRESSDB_ARTIFACT_ROOT="$ARTIFACT_ROOT"
export TEST_ARTIFACTS_ROOT="$ARTIFACT_ROOT"

OUT_DIR=${OUT_DIR:-"$ARTIFACT_ROOT/perf/health"}
RUN_ID=${RUN_ID:-health-$(date +%Y%m%d%H%M%S)}
mkdir -p "$OUT_DIR"

echo "Running k6 health benchmark against $TARGET"
TARGET="$TARGET" k6 run "$K6_SCRIPT" | tee "$OUT_DIR/${RUN_ID}.out"
