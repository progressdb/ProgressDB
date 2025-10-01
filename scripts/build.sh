
#!/usr/bin/env bash
set -euo pipefail

# Build wrapper for the server binary.
# Usage examples:
#   OUT=dist/progressdb CGO_ENABLED=0 GOOS=linux GOARCH=amd64 ./scripts/build.sh
#   ./scripts/build.sh           # builds to dist/progressdb

# Output path for the built binary
OUT=${OUT:-dist/progressdb}
# Package path to build (relative to repo root)
PKG=${PKG:-./server/cmd/progressdb}

mkdir -p "$(dirname "$OUT")"
echo "Building progressdb -> $OUT"

# Use repo-local caches to avoid writing to global caches
GOCACHE_DIR=$(pwd)/.gocache
GOMODCACHE_DIR=$(pwd)/.gocache_modules
mkdir -p "$GOCACHE_DIR" "$GOMODCACHE_DIR"

# Run the build from the package directory so module resolution is stable.
# Honor caller-specified GOOS/GOARCH/CGO settings.
(cd "$(dirname "$PKG")" && \
  GOCACHE="$GOCACHE_DIR" GOMODCACHE="$GOMODCACHE_DIR" GO111MODULE=on go build -o "$OLDPWD/$OUT" "$PKG")

echo "Built: $OUT"
