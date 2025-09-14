#!/usr/bin/env bash
set -euo pipefail

# Dev helper: build and run minikms + server locally with a test KEK file.
# Usage: ./scripts/dev-minikms.sh

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
KEY_DIR="$ROOT_DIR/.minikms"
KEY_FILE="$KEY_DIR/kek.hex"
DATA_DIR="$KEY_DIR/data"
SOCKET="/tmp/progressdb-minikms.sock"
BIN_DIR="$ROOT_DIR/bin"

mkdir -p "$KEY_DIR" "$DATA_DIR" "$BIN_DIR"

echo "[dev-minikms] using key file: $KEY_FILE"
if [ ! -f "$KEY_FILE" ]; then
  echo "[dev-minikms] generating test KEK..."
  openssl rand -hex 32 > "$KEY_FILE"
  chmod 600 "$KEY_FILE"
fi

echo "[dev-minikms] building binaries..."
(
  cd "$ROOT_DIR/minikms" && go build -o "$BIN_DIR/minikms" ./cmd/minikms
)
(
  cd "$ROOT_DIR/server" && go build -o "$BIN_DIR/progressdb" ./cmd/progressdb
)

rm -f "$SOCKET"

export PROGRESSDB_MINIKMS_MASTER_KEY_FILE="$KEY_FILE"
export PROGRESSDB_KMS_SOCKET="$SOCKET"
export PROGRESSDB_KMS_DATA_DIR="$DATA_DIR"
export PROGRESSDB_USE_ENCRYPTION="true"

echo "[dev-minikms] starting minikms (logs -> $KEY_DIR/minikms.log)"
"$BIN_DIR/minikms" --socket "$SOCKET" --data-dir "$DATA_DIR" > "$KEY_DIR/minikms.log" 2>&1 &
MINIKMS_PID=$!
echo "[dev-minikms] minikms pid=$MINIKMS_PID"

echo "[dev-minikms] waiting for socket $SOCKET"
for i in $(seq 1 50); do
  if [ -S "$SOCKET" ]; then
    break
  fi
  sleep 0.1
done
if [ ! -S "$SOCKET" ]; then
  echo "[dev-minikms] minikms failed to create socket. see logs:" 
  tail -n +1 "$KEY_DIR/minikms.log"
  kill $MINIKMS_PID || true
  exit 1
fi

echo "[dev-minikms] starting server (logs -> $KEY_DIR/server.log)"
"$BIN_DIR/progressdb" > "$KEY_DIR/server.log" 2>&1 &
SERVER_PID=$!
echo "[dev-minikms] server pid=$SERVER_PID"

cleanup() {
  echo "[dev-minikms] stopping..."
  kill "$SERVER_PID" "$MINIKMS_PID" 2>/dev/null || true
  wait "$SERVER_PID" 2>/dev/null || true
  wait "$MINIKMS_PID" 2>/dev/null || true
  rm -f "$SOCKET"
}

trap cleanup EXIT INT TERM

echo "[dev-minikms] dev environment running. To view logs:"
echo "  tail -f $KEY_DIR/minikms.log $KEY_DIR/server.log"
echo "Press Ctrl+C to stop and cleanup."

wait

