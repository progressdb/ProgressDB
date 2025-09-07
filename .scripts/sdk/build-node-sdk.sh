#!/bin/sh
set -eu

# Build the Node backend SDK (TypeScript -> dist)
# Usage: .scripts/build-node-sdk.sh [--install]

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
SDK_DIR=$(cd "$SCRIPT_DIR/../clients/sdk/backend/nodejs" && pwd)

cd "$SDK_DIR"

if [ "${1-}" = "--install" ]; then
  echo "Installing deps with npm ci..."
  npm ci
fi

echo "Building SDK in $SDK_DIR ..."
npm run build
echo "Build complete. Output: $SDK_DIR/dist"

