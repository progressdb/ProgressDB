#!/usr/bin/env bash
set -euo pipefail

# External-mode encrypted dev runner (per-mode folder). Does NOT auto-start KMS.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# project root is three levels up from scripts/enc/external
ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
TEMPLATE="$SCRIPT_DIR/config.template.yaml"
OUT_CFG="$SCRIPT_DIR/config.generated.yaml"

WAIT_KMS=0
WAIT_TIMEOUT=30

# Simple arg parsing for local options: --wait-kms [--wait-timeout N]
while [ $# -gt 0 ]; do
  case "$1" in
    --wait-kms)
      WAIT_KMS=1; shift ;;
    --wait-timeout)
      WAIT_TIMEOUT="$2"; shift 2 ;;
    --)
      shift; break ;;
    -*)
      # unknown flag: stop parsing and leave remaining args for server
      break ;;
    *)
      # positional arg: stop parsing and leave remaining args
      break ;;
  esac
done

if [[ ! -f "$TEMPLATE" ]]; then
  echo "Missing template: $TEMPLATE" >&2; exit 1
fi

MASTER_HEX="$(openssl rand -hex 32)"

# Generate server config embedding the master key
awk -v mk="$MASTER_HEX" '/master_key_hex:/ { sub(/master_key_hex: ".*"/, "master_key_hex: \"" mk "\"") } { print }' "$TEMPLATE" > "$OUT_CFG"

export PROGRESSDB_USE_ENCRYPTION=1
export PROGRESSDB_KMS_MODE=external
export PROGRESSDB_KMS_ENDPOINT=127.0.0.1:6820

echo "DEV (enc:external): using config $OUT_CFG"

echo "Note: KMS will NOT be started automatically. To start a local KMS for testing run:"
echo "    $ROOT_DIR/scripts/kms/dev.sh --endpoint 127.0.0.1:6820 --data-dir $ROOT_DIR/.kms/data"

wait_for_kms() {
  local timeout="$1"
  local start_ts=$(date +%s)
  local end_ts=$((start_ts + timeout))
  local url="http://127.0.0.1:6820/healthz"
  echo "Waiting for KMS health at $url (timeout ${timeout}s)..."
  while true; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      echo "KMS is healthy"
      return 0
    fi
    if [[ $(date +%s) -ge $end_ts ]]; then
      echo "Timed out waiting for KMS health" >&2
      return 1
    fi
    sleep 0.5
  done
}

if [[ $WAIT_KMS -eq 1 ]]; then
  if ! wait_for_kms "$WAIT_TIMEOUT"; then
    echo "KMS did not become healthy within ${WAIT_TIMEOUT}s" >&2
    exit 1
  fi
fi

cd "$ROOT_DIR/server"
mkdir -p .gopath/pkg/mod
export GOPATH="$PWD/.gopath"
export GOMODCACHE="$PWD/.gopath/pkg/mod"

exec go run ./cmd/progressdb --config "$OUT_CFG" "$@"
