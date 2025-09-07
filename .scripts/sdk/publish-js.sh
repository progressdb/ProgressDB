#!/usr/bin/env bash
set -euo pipefail

# Simple publisher: publishes to JSR, then builds for Node.js and publishes to npm

# Fix path: SDK_DIR should be relative to repo root, not .scripts
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
SDK_DIR="$ROOT_DIR/clients/sdk/frontend/typescript"

usage() {
  cat <<EOF
Usage: $(basename "$0") [--allow-slow-types]

Options:
  --allow-slow-types   Pass --allow-slow-types to jsr publish
  -h, --help           Show this help
EOF
}

ALLOW_SLOW=0

while [[ ${1:-} != "" ]]; do
  case "$1" in
    --allow-slow-types) ALLOW_SLOW=1; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1"; usage; exit 1;;
  esac
done

echo "Publishing SDK to JSR (Deno) registry..."
cd "$SDK_DIR"

if ! command -v npx >/dev/null 2>&1; then
  echo "npx not found. Install Node/npm to run jsr publish." >&2
  exit 2
fi

if [ "$ALLOW_SLOW" -eq 1 ]; then
  npx jsr publish --allow-slow-types
else
  npx jsr publish
fi
echo "JSR publish completed."

echo "Building for Node.js (npm)..."
npm install --no-audit --no-fund
npm run build

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found. Install Node/npm to publish to npm." >&2
  exit 2
fi

if ! npm whoami >/dev/null 2>&1; then
  echo "You are not logged in to npm. Run 'npm login' first." >&2
  exit 1
fi

echo "Publishing to npm..."
npm publish --access public
echo "npm publish completed."
