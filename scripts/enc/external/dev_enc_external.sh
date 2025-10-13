#!/usr/bin/env bash
set -euo pipefail

# Dev helper for "external" encryption mode.
# Typical usage: this script is invoked by scripts/dev.sh and may be passed
# the flags `--wait-kms` and `--wait-timeout N` followed by service args.
# Behavior:
#  - If `--wait-kms` is provided, wait up to N seconds (default 30) for
#    the KMS endpoint (127.0.0.1:6820) to accept TCP connections.
#  - After waiting (or immediately if not requested), exec the service.

if [ -z "${BASH_VERSION:-}" ]; then
  exec bash "$0" "$@"
fi

SCRIPT_PATH="${BASH_SOURCE[0]:-$0}"
SCRIPT_DIR="$(cd "$(dirname "$SCRIPT_PATH")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

WAIT_KMS=0
WAIT_TIMEOUT=30
ENDPOINT=${KMS_ENDPOINT:-127.0.0.1:6820}

# Parse our limited flags; leave remaining args for the service
PARSED_ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --wait-kms)
      WAIT_KMS=1; shift ;;
    --wait-timeout)
      WAIT_TIMEOUT="$2"; shift 2 ;;
    --)
      shift; break ;;
    -*)
      # unknown flag -> pass through
      PARSED_ARGS+=("$1"); shift ;;
    *)
      PARSED_ARGS+=("$1"); shift ;;
  esac
done

if [[ $WAIT_KMS -eq 1 ]]; then
  echo "[enc:external] Waiting up to $WAIT_TIMEOUT seconds for KMS at $ENDPOINT"
  HOST=${ENDPOINT%%:*}
  PORT=${ENDPOINT##*:}
  SECONDS_WAITED=0
  while true; do
    if (echo > /dev/tcp/$HOST/$PORT) >/dev/null 2>&1; then
      echo "[enc:external] KMS reachable at $ENDPOINT"
      break
    fi
    if [[ $SECONDS_WAITED -ge $WAIT_TIMEOUT ]]; then
      echo "[enc:external] timeout waiting for KMS ($ENDPOINT) after $WAIT_TIMEOUTs" >&2
      exit 2
    fi
    sleep 1
    SECONDS_WAITED=$((SECONDS_WAITED+1))
  done
fi

CFG="$ROOT_DIR/scripts/config.yaml"
echo "[enc:external] Launching service against external KMS endpoint $ENDPOINT"
cd "$ROOT_DIR/service"
export GOPATH="$PWD/.gopath"
export GOMODCACHE="$PWD/.gopath/pkg/mod"
exec go run ./cmd/progressdb --config "$CFG" "${PARSED_ARGS[@]}"

