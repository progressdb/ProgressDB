#!/usr/bin/env bash
set -euo pipefail

# Build the TypeScript SDK into JS (dist folder)
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SDK_DIR_DEFAULT="$ROOT_DIR/clients/sdk/frontend/typescript"

usage() {
  cat <<EOF
Usage: $(basename "$0") [--sdk-dir <path>] [--no-install]

Options:
  --sdk-dir <path>   Path to the SDK directory (default: $SDK_DIR_DEFAULT)
  --no-install       Skip npm install (assumes deps already installed)
  -h, --help         Show this help
EOF
}

SDK_DIR="$SDK_DIR_DEFAULT"
NO_INSTALL=0

while [[ ${1:-} != "" ]]; do
  case "$1" in
    --sdk-dir) SDK_DIR="$2"; shift 2;;
    --no-install) NO_INSTALL=1; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1"; usage; exit 1;;
  esac
done

echo "Building JS SDK in $SDK_DIR"
cd "$SDK_DIR"

if [ "$EUID" -eq 0 ]; then
  echo "Warning: running build as root (sudo) is not recommended." >&2
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found. Install Node/npm to build the SDK." >&2
  exit 2
fi

if [ $NO_INSTALL -eq 0 ]; then
  echo "Installing dependencies (npm install)"
  npm install --no-audit --no-fund
else
  echo "Skipping npm install (--no-install)"
fi

echo "Running npm run build"
npm run build

echo "Build complete. Output in $SDK_DIR/dist"
