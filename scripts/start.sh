#!/usr/bin/env bash
set -euo pipefail

# Start the progressdb server using a local module cache.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR/server"

mkdir -p .gopath/pkg/mod
export GOPATH="$PWD/.gopath"
export GOMODCACHE="$PWD/.gopath/pkg/mod"
export GOSUMDB=off

exec go run ./cmd/progressdb "$@"

