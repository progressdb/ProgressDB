#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SDK_DIR_DEFAULT="$ROOT_DIR/clients/sdk/frontend/typescript"

usage() {
  cat <<EOF
Usage: $(basename "$0") [--sdk-dir <path>] [--build-first] [--allow-slow-types]

Options:
  --sdk-dir <path>       Path to the SDK directory (default: $SDK_DIR_DEFAULT)
  --build-first          Run build before publish
  --allow-slow-types     Pass --allow-slow-types to jsr publish (skip slow type error blocking)
  -h, --help             Show this help
EOF
}

SDK_DIR="$SDK_DIR_DEFAULT"
BUILD_FIRST=0
ALLOW_SLOW_TYPES=0

while [[ ${1:-} != "" ]]; do
  case "$1" in
    --sdk-dir) SDK_DIR="$2"; shift 2;;
    --build-first) BUILD_FIRST=1; shift;;
    --allow-slow-types) ALLOW_SLOW_TYPES=1; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1"; usage; exit 1;;
  esac
done

cd "$SDK_DIR"

if [ $BUILD_FIRST -eq 1 ]; then
  echo "Building before publish"
  "$ROOT_DIR/.scripts/sdk/build-js-sdk.sh" --sdk-dir "$SDK_DIR"
fi

if [ ! -f mod.ts ]; then
  echo "mod.ts entrypoint not found in $SDK_DIR — ensure mod.ts exists and references your SDK exports." >&2
  exit 1
fi

if ! command -v npx >/dev/null 2>&1; then
  echo "npx not found. Install Node/npm to run jsr publish via npx." >&2
  exit 2
fi

echo "Publishing via jsr (npx jsr publish) — interactive login may open a browser."

JSR_ARGS=()
if [ $ALLOW_SLOW_TYPES -eq 1 ]; then
  JSR_ARGS+=("--allow-slow-types")
fi

# Run jsr publish which will prompt for interactive authentication
npx jsr publish "${JSR_ARGS[@]}" || {
  echo "jsr publish failed" >&2
  exit 1
}

echo "Published to jsr"
