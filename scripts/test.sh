#!/usr/bin/env bash
set -euo pipefail

echo "Running server tests..."
cd "$(dirname "$0")/.."

# Use a repo-local Go build cache to avoid permissions on CI/host caches
export GOCACHE="$(pwd)/.gocache"
mkdir -p "$GOCACHE"

# Run from the server directory so the module resolves correctly
cd server

# Prefer gotestsum for nicer output when available, otherwise fall back to go test
if command -v gotestsum >/dev/null 2>&1; then
  echo "Using gotestsum for formatted test output"
  gotestsum --format=pkgname --junitfile tests/junit.xml -- ./... -- -v
else
  echo "gotestsum not found; using go test"
  go test ./... -v
fi
