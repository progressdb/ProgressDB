#!/usr/bin/env bash
set -euo pipefail

# Dev helper for "embedded" encryption mode.
# Starts a local KMS (via scripts/kms/dev.sh) in the background, waits
# briefly for it to initialize, then runs the server in the foreground.
# On exit the KMS background process is killed.

if [ -z "${BASH_VERSION:-}" ]; then
  exec bash "$0" "$@"
fi

SCRIPT_PATH="${BASH_SOURCE[0]:-$0}"
SCRIPT_DIR="$(cd "$(dirname "$SCRIPT_PATH")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

KMS_HELPER="$ROOT_DIR/scripts/kms/dev.sh"
CFG="$ROOT_DIR/scripts/config.yaml"

if [[ ! -f "$KMS_HELPER" ]]; then
  echo "KMS dev helper not found: $KMS_HELPER" >&2
  exit 1
fi

echo "[enc:embedded] Starting development KMS (background)..."
# Start KMS helper in background. Use --no-build to speed iterations when
# the binary already exists; if this fails, fall back to invoking without it.
("$KMS_HELPER" --no-build >> "$ROOT_DIR/.kms/dev.log" 2>&1) &
KMS_PID=$!

trap 'echo "[enc:embedded] Stopping KMS (pid=$KMS_PID)"; kill "$KMS_PID" 2>/dev/null || true' EXIT

# Wait a short period for KMS to initialize; this is intentionally simple.
echo "[enc:embedded] Waiting for KMS to initialize..."
sleep 2

echo "[enc:embedded] Launching server (foreground)"
cd "$ROOT_DIR/server"
export GOPATH="$PWD/.gopath"
export GOMODCACHE="$PWD/.gopath/pkg/mod"
exec go run ./cmd/progressdb --config "$CFG" "$@"

