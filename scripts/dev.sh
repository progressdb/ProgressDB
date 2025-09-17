#!/usr/bin/env bash
set -euo pipefail

# Start the progressdb server using a local module cache.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR/server"

# Load env: prefer server/.env; fallback to root .env by exporting vars
if [[ -f .env ]]; then
  # The app will also load this, but sourcing is harmless
  set -a; . ./.env; set +a
elif [[ -f "$ROOT_DIR/.env" ]]; then
  set -a; . "$ROOT_DIR/.env"; set +a
fi

source "$ROOT_DIR/scripts/dev_env.sh"
DEV_CFG="$DEV_CFG"
echo "DEV: using config $DEV_CFG"

mkdir -p .gopath/pkg/mod
export GOPATH="$PWD/.gopath"
export GOMODCACHE="$PWD/.gopath/pkg/mod"
# export GOSUMDB=off

exec go run ./cmd/progressdb --config "$DEV_CFG" "$@"
