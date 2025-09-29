#!/usr/bin/env bash
set -euo pipefail

echo "Running E2E server tests..."
cd "$(dirname "$0")/.."

# Build server binary once and export for test helper to reuse
OUT_BIN="$PWD/.build/progressdb-test-bin"
mkdir -p "$(dirname "$OUT_BIN")"
echo "Building server binary -> $OUT_BIN"
go build -o "$OUT_BIN" ./server/cmd/progressdb
export PROGRESSDB_TEST_BINARY="$OUT_BIN"

echo "Running server E2E tests (real-process)..."
cd server
# include integration tag to ensure any integration-tagged tests run
go test -tags=integration ./tests -v
