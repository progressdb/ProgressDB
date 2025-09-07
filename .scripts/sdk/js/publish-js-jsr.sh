#!/usr/bin/env bash
set -euo pipefail

# Placeholder script for publishing to a JS registry other than npm (JSR).
# Adjust registry URL and auth as needed.
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SDK_DIR="$ROOT_DIR/clients/sdk/frontend/typescript"

REGISTRY_URL=""

cd "$SDK_DIR"

if [ -z "$REGISTRY_URL" ]; then
  echo "Set REGISTRY_URL in this script to publish to a custom registry." >&2
  exit 1
fi

if [ ! -d dist ]; then
  echo "dist not found — run build first: scripts/sdk/js/build-js-sdk.sh" >&2
  exit 1
fi

echo "Publishing to custom registry: $REGISTRY_URL"
npm publish --registry "$REGISTRY_URL" dist

