#!/usr/bin/env bash
set -euo pipefail

# Dev helper: build and run kms + server locally with a test KEK file.
# Usage: ./scripts/dev-kms.sh

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)

# refuse to run as root to avoid creating root-owned artifacts which break
# local iterative development. Run as your normal user.
if [ "$(id -u)" -eq 0 ]; then
  echo "Do not run this script as root; run as your normal user. Exiting."
  exit 1
fi
KEY_DIR="$ROOT_DIR/.kms"
KEY_FILE="$KEY_DIR/kek.hex"
DATA_DIR="$KEY_DIR/data"
SOCKET="/tmp/progressdb-kms.sock"
BIN_DIR="$ROOT_DIR/bin"

mkdir -p "$KEY_DIR" "$DATA_DIR" "$BIN_DIR"

echo "[dev-kms] using key file: $KEY_FILE"
if [ ! -f "$KEY_FILE" ]; then
  echo "[dev-kms] generating test KEK..."
  openssl rand -hex 32 > "$KEY_FILE"
  chmod 600 "$KEY_FILE"
fi

echo "[dev-kms] building binaries..."
(
  cd "$ROOT_DIR/kms" && go build -o "$BIN_DIR/kms" ./cmd/kms
)
(
  cd "$ROOT_DIR/server" && go build -o "$BIN_DIR/progressdb" ./cmd/progressdb
)

rm -f "$SOCKET"

export PROGRESSDB_KMS_MASTER_KEY_FILE="$KEY_FILE"
export PROGRESSDB_KMS_SOCKET="$SOCKET"
export PROGRESSDB_KMS_DATA_DIR="$DATA_DIR"
export PROGRESSDB_USE_ENCRYPTION="true"
export PROGRESSDB_KMS_ALLOWED_UIDS=$(id -u)

echo "[dev-kms] starting kms (logs -> $KEY_DIR/kms.log)"
"$BIN_DIR/kms" --socket "$SOCKET" --data-dir "$DATA_DIR" > "$KEY_DIR/kms.log" 2>&1 &
KMS_PID=$!
echo "[dev-kms] kms pid=$KMS_PID"

echo "[dev-kms] waiting for socket $SOCKET"
for i in $(seq 1 50); do
  if [ -S "$SOCKET" ]; then
    break
  fi
  sleep 0.1
done
if [ ! -S "$SOCKET" ]; then
  echo "[dev-kms] kms failed to create socket. see logs:" 
  tail -n +1 "$KEY_DIR/kms.log"
  kill $KMS_PID || true
  exit 1
fi

echo "[dev-kms] starting server (logs -> $KEY_DIR/server.log)"
"$BIN_DIR/progressdb" > "$KEY_DIR/server.log" 2>&1 &
SERVER_PID=$!
echo "[dev-kms] server pid=$SERVER_PID"

cleanup() {
  echo "[dev-kms] stopping..."
  kill "$SERVER_PID" "$KMS_PID" 2>/dev/null || true
  wait "$SERVER_PID" 2>/dev/null || true
  wait "$KMS_PID" 2>/dev/null || true
  rm -f "$SOCKET"
}

trap cleanup EXIT INT TERM

echo "[dev-kms] dev environment running. To view logs:"
echo "  tail -f $KEY_DIR/kms.log $KEY_DIR/server.log"
echo "Press Ctrl+C to stop and cleanup."

wait

