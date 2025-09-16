#!/usr/bin/env bash
set -euo pipefail

# Dev helper: build and run the KMS only with a test KEK file.
# Usage: ./scripts/kms/dev.sh [--no-build] [--kms-bin <path>] [--socket <path>] [--data-dir <path>] [--mkfile <path>]

# Ensure we're running under bash (re-exec if invoked via 'sh') so we can
# rely on ${BASH_SOURCE[0]} when available.
if [ -z "${BASH_VERSION:-}" ]; then
  exec bash "$0" "$@"
fi

# Determine script directory robustly, even when invoked via symlink.
SCRIPT_PATH="${BASH_SOURCE[0]:-$0}"
SCRIPT_DIR=$(cd "$(dirname "$SCRIPT_PATH")" && pwd)
# Project root is two levels up from scripts/kms/dev.sh (repo root)
ROOT_DIR=$(cd "$SCRIPT_DIR/../.." && pwd)

# Debugging/logging: print helpful environment and path info
echo "[dev-kms][debug] Current working directory: $(pwd)"
echo "[dev-kms][debug] Script location: $0"
echo "[dev-kms][debug] ROOT_DIR: $ROOT_DIR"
echo "[dev-kms][debug] User: $(whoami) (uid=$(id -u))"
echo "[dev-kms][debug] Shell: $SHELL"
echo "[dev-kms][debug] PATH: $PATH"

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
KEY_FILE_HEX=""
DATA_DIR="$KEY_DIR/data"

echo "[dev-kms][debug] BIN_DIR: $BIN_DIR"
echo "[dev-kms][debug] KMS_BIN: $KMS_BIN"
echo "[dev-kms][debug] SOCKET: $SOCKET"
echo "[dev-kms][debug] KEY_DIR: $KEY_DIR"
echo "[dev-kms][debug] DATA_DIR: $DATA_DIR"

mkdir -p "$KEY_DIR" "$DATA_DIR" "$BIN_DIR"

# Default: build the kms binary unless `--no-build` is passed.
BUILD=1
# Simple arg parsing: --no-build, --kms-bin <path>, --socket <path>, --data-dir <path>
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-build)
      BUILD=0; shift ;;
    --kms-bin)
      KMS_BIN="$2"; shift 2 ;;
    --socket)
      SOCKET="$2"; shift 2 ;;
    --data-dir)
      DATA_DIR="$2"; shift 2 ;;
    --help|-h)
      echo "Usage: $0 [--no-build] [--kms-bin <path>] [--socket <path>] [--data-dir <path>]"; exit 0 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

# generate test KEK hex if missing
if [ -z "$KEY_FILE_HEX" ]; then
  echo "[dev-kms] generating test KEK hex..."
  KEY_FILE_HEX=$(openssl rand -hex 32)
  echo "[dev-kms][debug] Generated KEY_FILE_HEX: (hidden)"
fi

# Build binary if missing
if [[ $BUILD -eq 1 ]]; then
  echo "[dev-kms] building kms binary..."
  echo "[dev-kms][debug] Building in $ROOT_DIR/kms"
  # Use a repo-local module cache to avoid writing to the global module cache
  GOCACHE_DIR="$ROOT_DIR/.gocache_modules"
  mkdir -p "$GOCACHE_DIR"
  (cd "$ROOT_DIR/kms" && GOMODCACHE="$GOCACHE_DIR" GO111MODULE=on go build -mod=mod -o "$KMS_BIN" ./cmd/kms)
  chmod +x "$KMS_BIN" || true
else
  echo "[dev-kms][debug] Skipping build; using $KMS_BIN"
fi

rm -f "$SOCKET" || true

# Write a self-contained YAML config consumed by KMS for startup secrets.
# The config embeds the master key hex directly so no extra secret files
# are required on disk.
CONFIG_FILE="$KEY_DIR/config.yaml"
cat > "$CONFIG_FILE" <<EOF
master_key_hex: "$KEY_FILE_HEX"
EOF

echo "[dev-kms][debug] Wrote config file: $CONFIG_FILE"
echo "[dev-kms][debug] Config file contents:"
cat "$CONFIG_FILE" | sed 's/master_key_hex:.*/master_key_hex: "<hidden>"/'

LOG_DIR="$KEY_DIR/logs"
mkdir -p "$LOG_DIR"
KMS_LOG="$LOG_DIR/kms.log"

echo "[dev-kms][debug] Log directory: $LOG_DIR"
echo "[dev-kms][debug] KMS log file: $KMS_LOG"

echo "[dev-kms] starting kms in foreground (config -> $CONFIG_FILE)"
echo "[dev-kms][debug] Launch command: $KMS_BIN --socket $SOCKET --data-dir $DATA_DIR --config $CONFIG_FILE"

# Run in foreground so logs appear in this terminal. KMS will receive
# signals (Ctrl+C) and perform graceful shutdown.
exec "$KMS_BIN" --socket "$SOCKET" --data-dir "$DATA_DIR" --config "$CONFIG_FILE"
