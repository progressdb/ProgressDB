#!/usr/bin/env bash
set -euo pipefail

echo "Running server tests..."
cd "$(dirname "$0")/.."

# Use a repo-local Go build cache to avoid permissions on CI/host caches
export GOCACHE="$(pwd)/.gocache"
mkdir -p "$GOCACHE"

# Run from the server directory so the module resolves correctly
cd server
go test ./... -v