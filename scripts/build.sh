#!/usr/bin/env bash
set -euo pipefail

# Build the progressdb binary into ./dist by default.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR/server"

mkdir -p .gopath/pkg/mod "$ROOT_DIR/dist"
export GOPATH="$PWD/.gopath"
export GOMODCACHE="$PWD/.gopath/pkg/mod"
export GOSUMDB=off

OUT="${OUT:-$ROOT_DIR/dist/progressdb}"

# Allow cross-compilation via GOOS/GOARCH; default CGO disabled for static-ish builds.
export CGO_ENABLED="${CGO_ENABLED:-0}"

# Optional build metadata: VERSION, COMMIT, BUILDDATE can be injected to ldflags.
LDFLAGS="-s -w"
if [ -n "${VERSION:-}" ]; then
  LDFLAGS="$LDFLAGS -X 'main.version=${VERSION}'"
fi
if [ -n "${COMMIT:-}" ]; then
  LDFLAGS="$LDFLAGS -X 'main.commit=${COMMIT}'"
fi
if [ -n "${BUILDDATE:-}" ]; then
  LDFLAGS="$LDFLAGS -X 'main.buildDate=${BUILDDATE}'"
fi

echo "Building to $OUT ..."
go build -trimpath -ldflags "$LDFLAGS" -o "$OUT" ./cmd/progressdb
echo "Done."
