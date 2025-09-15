#!/usr/bin/env bash
set -euo pipefail

# Dev helper: build and run KMS + server locally in a production-like way.
# Usage: ./scripts/dev-start-all.sh [--no-build] [--kms-bin <path>] [--server-bin <path>] [--socket <path>] [--data-dir <path>] [--mkfile <path>] [--foreground]

# --- Section: Set up default paths and variables ---
ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
BUILD=1
FOREGROUND=0
BIN_DIR="$ROOT_DIR/bin"
KMS_BIN="$BIN_DIR/kms"
SERVER_BIN="$BIN_DIR/progressdb"
SOCKET="/tmp/progressdb-kms.sock"
DATA_DIR="$ROOT_DIR/.dev-kms/data"
MKFILE="$ROOT_DIR/.dev-kms/kek.hex"

# --- Section: Parse command-line arguments ---
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-build) BUILD=0; shift ;; # Skip building binaries if --no-build is set
    --kms-bin) KMS_BIN="$2"; shift 2 ;; # Override KMS binary path
    --server-bin) SERVER_BIN="$2"; shift 2 ;; # Override server binary path
    --socket) SOCKET="$2"; shift 2 ;; # Override socket path
    --data-dir) DATA_DIR="$2"; shift 2 ;; # Override data directory
    --mkfile) MKFILE="$2"; shift 2 ;; # Override master key file path
    --foreground) FOREGROUND=1; shift ;; # Run server in foreground for debugging
    --help|-h) echo "Usage: $0 [--no-build] [--kms-bin <path>] [--server-bin <path>] [--socket <path>] [--data-dir <path>] [--mkfile <path>] [--foreground]"; exit 0 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

# --- Section: Create necessary directories and set permissions ---
mkdir -p "$BIN_DIR" "$DATA_DIR" "$(dirname "$MKFILE")"
chmod 700 "$DATA_DIR" || true

# --- Section: Set up log and pidfile paths ---
LOG_DIR="$ROOT_DIR/.dev-kms/logs"
mkdir -p "$LOG_DIR"
KMS_LOG="$LOG_DIR/kms.log"
SERVER_LOG="$LOG_DIR/server.log"
KMS_PIDFILE="$ROOT_DIR/.dev-kms/kms.pid"
SERVER_PIDFILE="$ROOT_DIR/.dev-kms/server.pid"

# --- Section: Build KMS and server binaries if needed ---
if [[ $BUILD -eq 1 ]]; then
  echo "Building KMS and server binaries..."
  (cd "$ROOT_DIR/kms" && go build -o "$KMS_BIN" ./cmd/kms)
  (cd "$ROOT_DIR/server" && go build -o "$SERVER_BIN" ./cmd/progressdb)
  chmod +x "$KMS_BIN" "$SERVER_BIN" || true
fi

# --- Section: Generate a dev master KEK if it doesn't exist ---
if [ ! -f "$MKFILE" ]; then
  echo "Generating dev master KEK at $MKFILE"
  openssl rand -hex 32 > "$MKFILE"
  chmod 600 "$MKFILE"
fi

# --- Section: Export environment variables for server and KMS ---
export PROGRESSDB_KMS_SOCKET="$SOCKET"
export PROGRESSDB_KMS_DATA_DIR="$DATA_DIR"
export PROGRESSDB_USE_ENCRYPTION="true"

# --- Section: Remove any stale socket file ---
rm -f "$SOCKET" || true

# --- Section: Start the server process (foreground or background) ---
echo "Starting server: $SERVER_BIN"
env | grep PROGRESSDB | sed -n '1,200p'
if [[ "$FOREGROUND" -eq 1 ]]; then
  # Run server in the foreground for interactive debugging. This will
  # replace the shell process with the server and print logs to stdout.
  exec "$SERVER_BIN"
else
  # Default: run server in background and redirect logs to file.
  "$SERVER_BIN" >> "$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  echo $SERVER_PID > "$SERVER_PIDFILE"
  echo "Server pid=$SERVER_PID (logs -> $SERVER_LOG)"
fi

# --- Section: Wait for the KMS socket to be created by the server ---
echo "Waiting for socket $SOCKET (created by server-spawned KMS)"
for i in $(seq 1 100); do
  if [ -S "$SOCKET" ]; then
    break
  fi
  sleep 0.1
done
if [ ! -S "$SOCKET" ]; then
  echo "KMS (spawned by server) failed to create socket. See server logs:"
  tail -n +1 "$SERVER_LOG"
  kill $SERVER_PID || true
  exit 1
fi
echo "KMS socket available: $SOCKET"

# --- Section: Cleanup function to stop server and remove socket on exit ---
cleanup() {
  echo "Stopping server and KMS..."
  kill "$SERVER_PID" 2>/dev/null || true
  wait "$SERVER_PID" 2>/dev/null || true
  rm -f "$SOCKET"
}

trap cleanup EXIT INT TERM

# --- Section: Final instructions and wait for background processes ---
echo "Dev environment running. To view logs:"
echo "  tail -f $KMS_LOG $SERVER_LOG"
echo "Press Ctrl+C to stop and cleanup."

wait
