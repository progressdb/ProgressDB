#!/usr/bin/env bash
set -euo pipefail

echo "Running server tests..."
cd "$(dirname "$0")/.."

# Use a repo-local Go build cache to avoid permissions on CI/host caches
export GOCACHE="$(pwd)/.gocache"
mkdir -p "$GOCACHE"

# Run from the server directory so the module resolves correctly
cd server

# Log file for JSON output
JSON_LOG="../logs/test-results.json"

# Prefer gotestsum for nicer output and JSON log when available, otherwise fall back to go test with tee to file
if command -v gotestsum >/dev/null 2>&1; then
  echo "Using gotestsum for formatted test output and JSON log"
  gotestsum --format=testdox --jsonfile "$JSON_LOG" -- ./... -- -v
else
  echo "gotestsum not found; using go test and logging output to $JSON_LOG"
  go test ./... -v | tee "$JSON_LOG"
fi
