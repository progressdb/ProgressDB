#!/usr/bin/env bash
set -euo pipefail

# Simple helper to build (optional) and start the kms binary.
# Usage: bash scripts/start-kms.sh [--build] [--bin <path>] [--endpoint <addr>] [--data-dir <path>]

BIN="./bin/kms"
ENDPOINT="127.0.0.1:6820"
DATA_DIR="./kms-data"
BUILD=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --build) BUILD=1; shift ;;
    --bin) BIN="$2"; shift 2 ;;
    --endpoint) ENDPOINT="$2"; shift 2 ;;
    --data-dir) DATA_DIR="$2"; shift 2 ;;
    --help|-h) echo "Usage: $0 [--build] [--bin <path>] [--endpoint <addr>] [--data-dir <path>]"; exit 0 ;;
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
  if ! go build -o "$BIN" ./kms/cmd/progressdb-kms; then
    echo "go build failed"
    exit 1
  fi
  chmod +x "$BIN" || true
fi

echo "Starting kms..."
echo " endpoint: $ENDPOINT"
echo " data-dir: $DATA_DIR"
echo " log: $LOGFILE"

# Ensure audit file exists with restricted perms
touch "$DATA_DIR/kms-audit.log" || true
chmod 600 "$DATA_DIR/kms-audit.log" || true

# Start in background and redirect logs
nohup "$BIN" --endpoint "$ENDPOINT" --data-dir "$DATA_DIR" >> "$LOGFILE" 2>&1 &
PID=$!
echo $PID > "$PIDFILE"
echo "kms started pid=$PID (logs -> $LOGFILE)"

echo "To stop: kill \$(cat $PIDFILE)"

exit 0
