#!/usr/bin/env bash
set -euo pipefail

# Run tests for Frontend SDK packages (typescript + react)
# Usage: ./scripts/sdk/test-frontend.sh [--unit|--integration|--all|--watch]

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
TS_DIR="$ROOT_DIR/clients/sdk/frontend/typescript"
REACT_DIR="$ROOT_DIR/clients/sdk/frontend/reactjs"

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

run_tests() {
  DIR="$1"
  TYPE="$2"
  echo "Running $TYPE tests in $DIR"
  cd "$DIR"
  npm ci
  if [[ $WATCH -eq 1 ]]; then
    if [[ $TYPE == "unit" ]]; then
      npm run test:unit -- --watch
    else
      npm run test:integration -- --watch
    fi
  else
    if [[ $TYPE == "unit" ]]; then
      npm run test:unit
    else
      npm run test:integration
    fi
  fi
}

case "$MODE" in
  unit)
    run_tests "$TS_DIR" unit
    run_tests "$REACT_DIR" unit || true
    ;;
  integration)
    run_tests "$TS_DIR" integration || true
    run_tests "$REACT_DIR" integration || true
    ;;
  all)
    run_tests "$TS_DIR" unit || true
    run_tests "$TS_DIR" integration || true
    run_tests "$REACT_DIR" unit || true
    run_tests "$REACT_DIR" integration || true
    ;;
esac

