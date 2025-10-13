#!/usr/bin/env bash
set -euo pipefail

###############################################################################
# ProgressDB Build Script
#
# This script builds the ProgressDB server binary for one or more target
# platforms. It supports embedding build metadata (version, commit, date)
# and uses local build caches to avoid polluting global Go caches.
#
# Usage examples:
#   OUT=dist/progressdb CGO_ENABLED=0 GOOS=linux GOARCH=amd64 ./scripts/build.sh
#   ./scripts/build.sh
#   TARGETS="darwin/amd64,linux/amd64" ./scripts/build.sh
#   HOST_ONLY=1 ./scripts/build.sh
#
# CLI flags:
#   -v version     Embed VERSION into binary (overrides env)
#   -c commit      Embed COMMIT into binary (overrides env)
#   -d build_date  Embed BUILD_DATE into binary (overrides env; default: now)
#   -h             Show usage
###############################################################################

# Output path for the built binary (default: dist/progressdb)
OUT=${OUT:-dist/progressdb}
# Package path to build (relative to repo root, default: ./service/cmd/progressdb)
PKG=${PKG:-./service/cmd/progressdb}

# Ensure output directory exists
mkdir -p "$(dirname "$OUT")"
echo "Building progressdb -> $OUT"

# Use repo-local Go build caches. If an artifact root is provided, keep
# caches under it so build outputs stay centralized.
if [ -n "${TEST_ARTIFACTS_ROOT:-}" ]; then
  ARTIFACT_ROOT="$TEST_ARTIFACTS_ROOT"
elif [ -n "${PROGRESSDB_ARTIFACT_ROOT:-}" ]; then
  ARTIFACT_ROOT="$PROGRESSDB_ARTIFACT_ROOT"
else
  ARTIFACT_ROOT=""
fi

if [ -n "$ARTIFACT_ROOT" ]; then
  mkdir -p "$ARTIFACT_ROOT"
  ARTIFACT_ROOT="$(cd "$ARTIFACT_ROOT" && pwd)"
  GOCACHE_DIR="$ARTIFACT_ROOT/cache/go-build"
  GOMODCACHE_DIR="$ARTIFACT_ROOT/cache/go-mod"
else
  GOCACHE_DIR="$(pwd)/.gocache"
  GOMODCACHE_DIR="$(pwd)/.gocache_modules"
fi
mkdir -p "$GOCACHE_DIR" "$GOMODCACHE_DIR"

# Determine package directory and base (for stable module resolution)
pkg_dir="$(dirname "$PKG")"
pkg_base="$(basename "$PKG")"

# Default target list for multi-platform builds
DEFAULT_TARGETS="darwin/amd64,darwin/arm64,linux/amd64,windows/amd64"

# Determine build targets:
# - If HOST_ONLY=1, build only for the current host platform
# - Otherwise, use TARGETS env var or default to DEFAULT_TARGETS
if [ "${HOST_ONLY:-0}" = "1" ]; then
  host_os=$(go env GOOS)
  host_arch=$(go env GOARCH)
  TARGETS="$host_os/$host_arch"
else
  TARGETS=${TARGETS:-$DEFAULT_TARGETS}
fi

# Output directory and base name for built binaries
out_dir=$(dirname "$OUT")
base_name=$(basename "$OUT")
mkdir -p "$out_dir"

# Print usage/help
usage() {
  echo "Usage: $0 [-v version] [-c commit] [-d build_date]" >&2
  echo "  -v version     Embed VERSION into binary (overrides env)" >&2
  echo "  -c commit      Embed COMMIT into binary (overrides env)" >&2
  echo "  -d build_date  Embed BUILD_DATE into binary (overrides env); if omitted it's auto-generated" >&2
  echo "  -h             Show this help message" >&2
  exit 1
}

# Parse CLI flags for build metadata
while getopts ":v:c:d:h" opt; do
  case $opt in
    v) VERSION="$OPTARG" ;;
    c) COMMIT="$OPTARG" ;;
    d) BUILD_DATE="$OPTARG" ;;
    h) usage ;;
    \?) echo "Invalid option: -$OPTARG" >&2; usage ;;
    :)  echo "Option -$OPTARG requires an argument." >&2; usage ;;
  esac
done

# Set BUILD_DATE to current UTC time if not provided
if [ -z "${BUILD_DATE:-}" ]; then
  BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
fi

# If VERSION is set, construct Go ldflags to embed build metadata
# (see main.version, main.commit, main.buildDate in Go code)
LDFLAGS=""
if [ -n "${VERSION:-}" ]; then
  LDFLAGS="-X main.version=${VERSION} -X main.commit=${COMMIT:-none} -X main.buildDate=${BUILD_DATE:-unknown}"
fi

# Save current working directory for output path resolution
OLDPWD=$(pwd)

# Build for each target in TARGETS (comma-separated "os/arch" pairs)
IFS=',' read -ra TARGET_ARR <<< "$TARGETS"
for t in "${TARGET_ARR[@]}"; do
  goos=${t%/*}
  goarch=${t#*/}
  ext=""
  # Add .exe extension for Windows targets
  if [ "$goos" = "windows" ]; then
    ext=".exe"
  fi
  outpath="$out_dir/${base_name}-${goos}-${goarch}${ext}"
  echo "Building $base_name for $goos/$goarch -> $outpath"
  (
    # Change to the package directory for stable module resolution
    cd "$pkg_dir/$pkg_base"
    # Set Go build environment variables and build
    GOCACHE="$GOCACHE_DIR" GOMODCACHE="$GOMODCACHE_DIR" GO111MODULE=on \
      CGO_ENABLED=${CGO_ENABLED:-0} GOOS=$goos GOARCH=$goarch \
      go build ${LDFLAGS:+-ldflags "$LDFLAGS"} -o "$OLDPWD/$outpath" .
  )
done

echo "Build complete."
echo "Built: $OUT (multi-target)"
echo "Targets: $TARGETS"
