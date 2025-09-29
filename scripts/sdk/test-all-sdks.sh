#!/usr/bin/env bash
set -euo pipefail

# Run all SDK tests (node + frontend). For CI or local convenience.
# Usage: ./scripts/sdk/test-all-sdks.sh [--watch]

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)

if [[ ${1-} == --watch ]]; then
  echo "Starting all SDK tests in watch mode"
  "$ROOT_DIR/scripts/sdk/test-node.sh" --all --watch &
  "$ROOT_DIR/scripts/sdk/test-frontend.sh" --all --watch &
  # run python tests once (no watch available)
  "$ROOT_DIR/scripts/sdk/test-python.sh"
  wait
else
  echo "Running all SDK tests"
  "$ROOT_DIR/scripts/sdk/test-node.sh" --all
  "$ROOT_DIR/scripts/sdk/test-python.sh"
  "$ROOT_DIR/scripts/sdk/test-frontend.sh" --all
fi
