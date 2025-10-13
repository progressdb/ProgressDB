#!/usr/bin/env bash
set -euo pipefail

echo "Running service tests..."
cd "$(dirname "$0")/.."

# Route all test artifacts into a shared root so logs and caches stay tidy
ARTIFACT_ROOT=${TEST_ARTIFACTS_ROOT:-${PROGRESSDB_ARTIFACT_ROOT:-"$(pwd)/tests/artifacts"}}
mkdir -p "$ARTIFACT_ROOT"
ARTIFACT_ROOT="$(cd "$ARTIFACT_ROOT" && pwd)"
export PROGRESSDB_ARTIFACT_ROOT="$ARTIFACT_ROOT"
export TEST_ARTIFACTS_ROOT="$ARTIFACT_ROOT"

# Use a repo-local Go build cache under the artifact root
export GOCACHE="$ARTIFACT_ROOT/cache/go"
mkdir -p "$GOCACHE"

# Run from the service directory so the module resolves correctly
cd service

# Log file for JSON output
JSON_LOG="$ARTIFACT_ROOT/logs/service-tests.jsonl"

# Ensure the logs directory exists so `tee` can write to the JSON log
mkdir -p "$(dirname "$JSON_LOG")"

# Prefer gotestsum for nicer output and JSON log when available, otherwise fall back to go test with tee to file
if command -v gotestsum >/dev/null 2>&1; then
  echo "Using gotestsum for formatted test output and JSON log"
  gotestsum --format=testdox --jsonfile "$JSON_LOG" -- ./... -- -v
else
  echo "gotestsum not found; using go test and logging output to $JSON_LOG"
  go test ./... -v | tee "$JSON_LOG"
fi
