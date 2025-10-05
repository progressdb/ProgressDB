so 
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
pkg_dir="$(dirname "$PKG")"
pkg_base="$(basename "$PKG")"

# By default build for multiple targets and place outputs under `dist/`.
# Override with HOST_ONLY=1 to build only for the current host platform.
# To customize targets set TARGETS as a comma-separated list of "os/arch" pairs.
# Examples:
#   TARGETS="darwin/amd64,linux/amd64" ./scripts/build.sh
#   HOST_ONLY=1 ./scripts/build.sh
# Default target list when doing multi-target builds:
DEFAULT_TARGETS="darwin/amd64,darwin/arm64,linux/amd64,windows/amd64"

if [ "${HOST_ONLY:-0}" = "1" ]; then
  host_os=$(go env GOOS)
  host_arch=$(go env GOARCH)
  TARGETS="$host_os/$host_arch"
else
  TARGETS=${TARGETS:-$DEFAULT_TARGETS}
fi

out_dir=$(dirname "$OUT")
base_name=$(basename "$OUT")
mkdir -p "$out_dir"

OLDPWD=$(pwd)
IFS=','; for t in $TARGETS; do
  goos=${t%/*}
  goarch=${t#*/}
  ext=""
  if [ "$goos" = "windows" ]; then
    ext=".exe"
  fi
  outpath="$out_dir/${base_name}-${goos}-${goarch}${ext}"
  echo "Building $base_name for $goos/$goarch -> $outpath"
  (
    cd "$pkg_dir/$pkg_base"
    GOCACHE="$GOCACHE_DIR" GOMODCACHE="$GOMODCACHE_DIR" GO111MODULE=on \
      CGO_ENABLED=${CGO_ENABLED:-0} GOOS=$goos GOARCH=$goarch \
      go build -o "$OLDPWD/$outpath" .
  )
done

echo "Built: $OUT (multi-target)
Targets: $TARGETS"
