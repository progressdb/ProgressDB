#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SDK_DIR_DEFAULT="$ROOT_DIR/clients/sdk/frontend/typescript"

usage() {
  cat <<EOF
Usage: $(basename "$0") [--sdk-dir <path>] [--build-first] [--dry-run]

Options:
  --sdk-dir <path>   Path to the SDK directory (default: $SDK_DIR_DEFAULT)
  --build-first      Run build before publishing
  --dry-run          Show what would be published without actually publishing
  -h, --help         Show this help
EOF
}

SDK_DIR="$SDK_DIR_DEFAULT"
BUILD_FIRST=0
DRY_RUN=0

while [[ ${1:-} != "" ]]; do
  case "$1" in
    --sdk-dir) SDK_DIR="$2"; shift 2;;
    --build-first) BUILD_FIRST=1; shift;;
    --dry-run) DRY_RUN=1; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1"; usage; exit 1;;
  esac
done

cd "$SDK_DIR"

if [ $BUILD_FIRST -eq 1 ]; then
  echo "Building before publish"
  "$ROOT_DIR/.scripts/sdk/build-js-sdk.sh" --sdk-dir "$SDK_DIR"
fi

if [ ! -d dist ]; then
  echo "dist not found — run build first: .scripts/sdk/build-js-sdk.sh" >&2
  exit 1
fi

PKG_NAME=$(node -e "console.log(require('./package.json').name)")
PKG_VER=$(node -e "console.log(require('./package.json').version)")

echo "Preparing to publish $PKG_NAME@$PKG_VER from $SDK_DIR"

if [ $DRY_RUN -eq 1 ]; then
  echo "Dry run: showing npm pack contents"
  npm pack --dry-run
  echo "Dry run complete. No publish performed."
  exit 0
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found. Install Node/npm to publish the SDK." >&2
  exit 2
fi

if ! npm whoami >/dev/null 2>&1; then
  echo "You are not logged in to npm. Run 'npm login' first." >&2
  exit 1
fi

echo "Publishing $PKG_NAME@$PKG_VER to npm (this will use the SDK package.json and include dist/ per 'files')"
npm publish --access public || {
  echo "npm publish failed" >&2
  exit 1
}

echo "Published $PKG_NAME@$PKG_VER"
