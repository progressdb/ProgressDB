#!/usr/bin/env bash
set -euo pipefail

# Publish to a JSR (Deno) registry using `npx jsr publish`.
# This script expects the SDK to include an `exports` field and a `mod.ts` entrypoint.
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SDK_DIR="$ROOT_DIR/clients/sdk/frontend/typescript"

cd "$SDK_DIR"

if [ ! -f mod.ts ]; then
  echo "mod.ts entrypoint not found in $SDK_DIR — ensure mod.ts exists and references your SDK exports." >&2
  exit 1
fi

if ! command -v npx >/dev/null 2>&1; then
  echo "npx not found. Install Node/npm to run jsr publish via npx." >&2
  exit 2
fi

echo "Publishing @progressdb/js via jsr (npx jsr publish) — interactive login may open a browser."

# Run jsr publish which will prompt for interactive authentication
npx jsr publish || {
  echo "jsr publish failed" >&2
  exit 1
}

echo "Published to jsr"

