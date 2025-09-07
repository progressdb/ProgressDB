#!/usr/bin/env bash
set -euo pipefail

# Build the TypeScript SDK into JS (dist folder)
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SDK_DIR="$ROOT_DIR/clients/sdk/frontend/typescript"

echo "Building JS SDK in $SDK_DIR"
cd "$SDK_DIR"

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found. Install Node/npm to build the SDK." >&2
  exit 2
fi

npm install --no-audit --no-fund
npm run build

echo "Build complete. Output in $SDK_DIR/dist"

