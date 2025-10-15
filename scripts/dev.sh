#!/usr/bin/env bash
# Ensure we run under bash (arrays and ${BASH_SOURCE} required). If invoked with
# sh, re-exec under bash so `set -u` and array usages work correctly.
if [ -z "${BASH_VERSION:-}" ]; then
  # If invoked via `sh scripts/dev.sh` then $0 == "sh" and the script path is
  # in $1. Shift so the script path becomes $0 when re-execing under bash.
  if [ "$0" = "sh" ] && [ "$#" -ge 1 ]; then
    shift
    exec bash "$@"
  else
    exec bash "$0" "$@"
  fi
fi
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "Starting development service..."
CFG="$ROOT_DIR/scripts/config.yaml"
cd "$ROOT_DIR/service"
mkdir -p .gopath/pkg/mod
export GOPATH="$PWD/.gopath"
export GOMODCACHE="$PWD/.gopath/pkg/mod"
exec go run ./cmd/progressdb --config "$CFG" "$@"
