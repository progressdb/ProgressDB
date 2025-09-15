#!/usr/bin/env bash
set -euo pipefail

# Dev helper: build and run the KMS only with a test KEK file.
# Usage: ./scripts/kms/dev.sh [--no-build] [--kms-bin <path>] [--socket <path>] [--data-dir <path>] [--mkfile <path>]

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)

# refuse to run as root to avoid creating root-owned artifacts which break
# local iterative development. Run as your normal user.
if [ "$(id -u)" -eq 0 ]; then
  echo "Do not run this script as root; run as your normal user. Exiting."
  exit 1
fi

# Defaults (override via CLI args)
BIN_DIR="$ROOT_DIR/bin"
KMS_BIN="$BIN_DIR/kms"
SOCKET="/tmp/progressdb-kms.sock"
KEY_DIR="$ROOT_DIR/.kms"
KEY_FILE="$KEY_DIR/kek.hex"
DATA_DIR="$KEY_DIR/data"
BUILD=1

mkdir -p "$KEY_DIR" "$DATA_DIR" "$BIN_DIR"

# Parse args
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-build) BUILD=0; shift ;;
    --kms-bin) KMS_BIN="$2"; shift 2 ;;
    --socket) SOCKET="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --mkfile) KEY_FILE="$2"; shift 2 ;;
    --help|-h) echo "Usage: $0 [--no-build] [--kms-bin <path>] [--socket <path>] [--data-dir <path>] [--mkfile <path>]"; exit 0 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

echo "[dev-kms] using key file: $KEY_FILE"
if [ ! -f "$KEY_FILE" ]; then
  echo "[dev-kms] generating test KEK..."
  openssl rand -hex 32 > "$KEY_FILE"
  chmod 600 "$KEY_FILE"
fi

if [[ $BUILD -eq 1 ]]; then
  echo "[dev-kms] building kms binary..."
  # Force module mode to ensure builds run inside module directories
  (cd "$ROOT_DIR/kms" && GO111MODULE=on go build -mod=mod -o "$KMS_BIN" ./cmd/kms)
  chmod +x "$KMS_BIN" || true
fi

rm -f "$SOCKET" || true

export PROGRESSDB_KMS_MASTER_KEY_FILE="$KEY_FILE"
export PROGRESSDB_KMS_SOCKET="$SOCKET"
export PROGRESSDB_KMS_DATA_DIR="$DATA_DIR"
export PROGRESSDB_USE_ENCRYPTION="true"
export PROGRESSDB_KMS_ALLOWED_UIDS="$(id -u)"

LOG_DIR="$KEY_DIR/logs"
mkdir -p "$LOG_DIR"
KMS_LOG="$LOG_DIR/kms.log"

echo "[dev-kms] starting kms (logs -> $KMS_LOG)"
"$KMS_BIN" --socket "$SOCKET" --data-dir "$DATA_DIR" > "$KMS_LOG" 2>&1 &
KMS_PID=$!
echo "[dev-kms] kms pid=$KMS_PID"

echo "[dev-kms] waiting for socket $SOCKET"
for i in $(seq 1 100); do
  if [ -S "$SOCKET" ]; then
    break
  fi
  sleep 0.1
done
if [ ! -S "$SOCKET" ]; then
  echo "[dev-kms] kms failed to create socket. see logs:" 
  tail -n +1 "$KMS_LOG"
  kill "$KMS_PID" || true
  exit 1
fi

cleanup() {
  echo "[dev-kms] stopping..."
  if [[ -n "${KMS_PID-}" ]]; then
    kill "$KMS_PID" 2>/dev/null || true
    wait "$KMS_PID" 2>/dev/null || true
  fi
  rm -f "$SOCKET" || true
}

trap cleanup EXIT INT TERM

echo "[dev-kms] dev KMS running. To view logs:"
echo "  tail -f $KMS_LOG"
echo "Press Ctrl+C to stop and cleanup."

wait

