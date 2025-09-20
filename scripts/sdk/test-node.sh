#!/usr/bin/env bash
set -euo pipefail

# Run tests for Node SDK packages (backend + related Node SDKs).
# Usage: ./scripts/sdk/test-node.sh [--unit|--integration|--all|--watch]

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
PKG_DIR="$ROOT_DIR/clients/sdk/backend/nodejs"

MODE=all
WATCH=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --unit) MODE=unit; shift ;;
    --integration) MODE=integration; shift ;;
    --all) MODE=all; shift ;;
    --watch) WATCH=1; shift ;;
    -h|--help) echo "Usage: $0 [--unit|--integration|--all] [--watch]"; exit 0 ;;
    *) echo "Unknown arg: $1"; exit 2 ;;
  esac
done

echo "Running Node SDK tests (mode=$MODE) in $PKG_DIR"
cd "$PKG_DIR"

if [[ ! -f package.json ]]; then
  echo "package.json not found in $PKG_DIR" >&2
  exit 2
fi

echo "Installing dev dependencies..."
npm ci

if [[ $WATCH -eq 1 ]]; then
  case "$MODE" in
    unit) npm run test:unit -- --watch ;;
    integration) npm run test:integration -- --watch ;;
    all) npm test -- --watch ;;
  esac
else
  case "$MODE" in
    unit) npm run test:unit -- --run ;;
    integration) npm run test:integration -- --run ;;
    all) npm test -- --run ;;
  esac
fi
