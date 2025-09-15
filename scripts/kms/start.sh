#!/usr/bin/env bash
set -euo pipefail

# Simple helper to build (optional) and start the kms binary.
# Usage: bash scripts/start-kms.sh [--build] [--bin <path>] [--socket <path>] [--data-dir <path>]

BIN="./bin/kms"
SOCKET="/tmp/progressdb-kms.sock"
DATA_DIR="./kms-data"
BUILD=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --build) BUILD=1; shift ;;
    --bin) BIN="$2"; shift 2 ;;
    --socket) SOCKET="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --help|-h) echo "Usage: $0 [--build] [--bin <path>] [--socket <path>] [--data-dir <path>]"; exit 0 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

mkdir -p "$(dirname "$BIN")"
mkdir -p "$DATA_DIR"
chmod 700 "$DATA_DIR" || true

LOGFILE="$DATA_DIR/kms.log"
PIDFILE="$DATA_DIR/kms.pid"

if [[ $BUILD -eq 1 || ! -x "$BIN" ]]; then
  echo "Building kms to $BIN..."
  # Build the kms binary; this assumes you have Go toolchain available.
  if ! go build -o "$BIN" ./kms/cmd/kms; then
    echo "go build failed"
    exit 1
  fi
  chmod +x "$BIN" || true
fi

echo "Starting kms..."
echo " socket: $SOCKET"
echo " data-dir: $DATA_DIR"
echo " log: $LOGFILE"

# Ensure audit file exists with restricted perms
touch "$DATA_DIR/kms-audit.log" || true
chmod 600 "$DATA_DIR/kms-audit.log" || true

# Remove stale socket if present
rm -f "$SOCKET" || true

# Start in background and redirect logs
nohup "$BIN" --socket "$SOCKET" --data-dir "$DATA_DIR" >> "$LOGFILE" 2>&1 &
PID=$!
echo $PID > "$PIDFILE"
echo "kms started pid=$PID (logs -> $LOGFILE)"

echo "To stop: kill \$(cat $PIDFILE) && rm -f $SOCKET"

exit 0

