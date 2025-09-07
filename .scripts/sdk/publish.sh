#!/usr/bin/env bash
set -euo pipefail

# Simple wrapper to build and publish the SDK. Use subcommands:
#   ./publish.sh build        -> build only
#   ./publish.sh publish-npm  -> build (optional) and publish to npm
#   ./publish.sh publish-jsr  -> build (optional) and publish to jsr
#   ./publish.sh publish-all  -> build then publish to both

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

usage() {
  cat <<EOF
Usage: $(basename "$0") <subcommand> [--no-build] [--dry-run]

Subcommands:
  build         Run the SDK build
  publish-npm   Publish to npm
  publish-jsr   Publish to JSR/Deno registry
  publish-all   Publish to both npm and jsr

Options:
  --no-build    Skip the build step
  --dry-run     When publishing to npm, run a dry-run pack instead of publish
  -h, --help    Show this help
EOF
}

if [ $# -lt 1 ]; then
  usage; exit 1
fi

SUBCMD=$1; shift
NO_BUILD=0
DRY_RUN=0

while [[ ${1:-} != "" ]]; do
  case "$1" in
    --no-build) NO_BUILD=1; shift;;
    --dry-run) DRY_RUN=1; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1"; usage; exit 1;;
  esac
done

if [ $NO_BUILD -eq 0 ]; then
  echo "Building SDK..."
  "$ROOT_DIR/.scripts/sdk/build-js-sdk.sh"
fi

case "$SUBCMD" in
  build)
    echo "Build complete." ;;
  publish-npm)
    if [ $DRY_RUN -eq 1 ]; then
      "$ROOT_DIR/.scripts/sdk/publish-js-npm.sh" --build-first --dry-run
    else
      "$ROOT_DIR/.scripts/sdk/publish-js-npm.sh" --build-first
    fi
    ;;
  publish-jsr)
    "$ROOT_DIR/.scripts/sdk/publish-js-jsr.sh" --build-first
    ;;
  publish-all)
    "$ROOT_DIR/.scripts/sdk/publish-js-npm.sh" --build-first
    "$ROOT_DIR/.scripts/sdk/publish-js-jsr.sh" --build-first
    ;;
  *) echo "Unknown subcommand: $SUBCMD"; usage; exit 1;;
esac

