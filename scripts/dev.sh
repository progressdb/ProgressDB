#!/usr/bin/env bash
# Ensure we run under bash (arrays and ${BASH_SOURCE} required). If invoked with
# sh, re-exec under bash so `set -u` and array usages work correctly.
if [ -z "${BASH_VERSION:-}" ]; then
  exec bash "$0" "$@"
fi
set -euo pipefail

# Interactive dev launcher. Prompts whether to enable encryption and which mode,
# then delegates to the appropriate script under scripts/enc/ or runs plain server.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Non-interactive options: --enc --mode <embedded|external> --wait-kms --wait-timeout N
USE_ENC=0
MODE="embedded"
WAIT_KMS=0
WAIT_TIMEOUT=30

# If the current shell does not support bash arrays (e.g. invoked via `sh`),
# re-exec the script under `bash` so array usage later works.
if ! ( eval 'tmp=()' ) 2>/dev/null; then
  exec bash "$0" "$@"
fi

while [ $# -gt 0 ]; do
  case "$1" in
    --enc)
      USE_ENC=1; shift ;;
    --no-enc)
      USE_ENC=0; shift ;;
    --mode)
      MODE="$2"; shift 2 ;;
    --wait-kms)
      WAIT_KMS=1; shift ;;
    --wait-timeout)
      WAIT_TIMEOUT="$2"; shift 2 ;;
    --)
      shift; break ;;
    -*)
      # unknown flag: stop parsing and leave remaining args for the server
      break ;;
    *)
      # positional -> leave for server
      break ;;
  esac
done
# Remaining "$@" are forwarded to the server command

if [[ $USE_ENC -eq 0 ]]; then
  # If no flags provided and no forwarded server args, run interactive prompt
  if [[ $# -eq 0 && $USE_ENC -eq 0 ]]; then
    echo "Start development server"
    read -r -p "Enable encryption (y/N)? " ENC_ANS
    ENC_ANS="${ENC_ANS:-n}"
    if [[ "$ENC_ANS" =~ ^([yY][eE][sS]|[yY])$ ]]; then
      read -r -p "KMS mode — embedded or external? (e/x) [e]: " MODE_ANS
      MODE_ANS="${MODE_ANS:-e}"
      if [[ "$MODE_ANS" =~ ^([eE])$ ]]; then
        MODE="embedded"
        USE_ENC=1
      else
        MODE="external"
        USE_ENC=1
      fi
    fi
  fi
fi

if [[ $USE_ENC -eq 1 ]]; then
  if [[ "$MODE" == "embedded" ]]; then
    exec "$ROOT_DIR/scripts/enc/embedded/dev_enc_embedded.sh" "$@"
  else
    # external mode: forward wait options if set
    if [[ $WAIT_KMS -eq 1 ]]; then
      exec "$ROOT_DIR/scripts/enc/external/dev_enc_external.sh" --wait-kms --wait-timeout "$WAIT_TIMEOUT" "$@"
    else
      exec "$ROOT_DIR/scripts/enc/external/dev_enc_external.sh" "$@"
    fi
  fi
else
  echo "Running plain (no encryption) dev..."
  CFG="$ROOT_DIR/scripts/config.yaml"
  cd "$ROOT_DIR/server"
  mkdir -p .gopath/pkg/mod
  export GOPATH="$PWD/.gopath"
  export GOMODCACHE="$PWD/.gopath/pkg/mod"
  exec go run ./cmd/progressdb --config "$CFG" "$@"
fi
