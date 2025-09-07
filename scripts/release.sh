#!/usr/bin/env bash
set -euo pipefail

# Release script: builds multi-platform binaries, packages them and writes checksums.
# Usage: scripts/release.sh [--version vX.Y.Z]

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="$ROOT_DIR/dist"
mkdir -p "$OUT_DIR"

VERSION="${1:-}"
if [ -n "$VERSION" ]; then
  # allow passing v1.2.3 or 1.2.3
  VERSION="${VERSION#v}"
fi

COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
BUILDDATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "Release build. version=$VERSION commit=$COMMIT date=$BUILDDATE"

matrix=(
  "linux amd64"
  "linux arm64"
  "darwin arm64"
  "windows amd64"
)

for entry in "${matrix[@]}"; do
  set -- $entry
  GOOS=$1
  GOARCH=$2
  echo "Building for $GOOS/$GOARCH"
  OUT_NAME="progressdb"
  EXT=""
  if [ "$GOOS" = "windows" ]; then
    EXT=".exe"
    OUT_NAME="progressdb.exe"
  fi
  OUT_PATH="$OUT_DIR/progressdb-${VERSION:-dev}-$GOOS-$GOARCH$EXT"

  # Export env for scripts/build.sh to pick up
  (cd "$ROOT_DIR" \
    && OUT="$OUT_PATH" CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    VERSION="$VERSION" COMMIT="$COMMIT" BUILDDATE="$BUILDDATE" \
    scripts/build.sh)

  # Package
  pushd "$OUT_DIR" >/dev/null
  if [ "$GOOS" = "windows" ]; then
    zip -9 "progressdb-${VERSION:-dev}-$GOOS-$GOARCH.zip" "$(basename "$OUT_PATH")" >/dev/null
    rm -f "progressdb-${VERSION:-dev}-$GOOS-$GOARCH.sha256"
    shasum -a 256 "progressdb-${VERSION:-dev}-$GOOS-$GOARCH.zip" > "progressdb-${VERSION:-dev}-$GOOS-$GOARCH.sha256"
  else
    tar -czf "progressdb-${VERSION:-dev}-$GOOS-$GOARCH.tar.gz" "$(basename "$OUT_PATH")"
    rm -f "progressdb-${VERSION:-dev}-$GOOS-$GOARCH.sha256"
    shasum -a 256 "progressdb-${VERSION:-dev}-$GOOS-$GOARCH.tar.gz" > "progressdb-${VERSION:-dev}-$GOOS-$GOARCH.sha256"
  fi
  popd >/dev/null
done

echo "Artifacts in $OUT_DIR"
ls -lah "$OUT_DIR"
