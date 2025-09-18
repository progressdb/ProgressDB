#!/usr/bin/env bash
set -euo pipefail

# Interactive dev launcher. Prompts whether to enable encryption and which mode,
# then delegates to the appropriate script under scripts/enc/ or runs plain server.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Non-interactive options: --enc --mode <embedded|external> --wait-kms --wait-timeout N
USE_ENC=0
MODE="embedded"
WAIT_KMS=0
WAIT_TIMEOUT=30

POSITIONAL=()
while [[ $# -gt 0 ]]; do
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
      shift; POSITIONAL+=("$@"); break ;;
    -*|--*)
      # unknown flag; forward to server
      POSITIONAL+=("$1"); shift ;;
    *) POSITIONAL+=("$1"); shift ;;
  esac
done
set -- "${POSITIONAL[@]}"

if [[ $USE_ENC -eq 0 ]]; then
  # If no flags provided, run interactive prompt
  if [[ ${#POSITIONAL[@]} -eq 0 && $USE_ENC -eq 0 ]]; then
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
  exec go run ./cmd/progressdb --config "$CFG" "${POSITIONAL[@]:+${POSITIONAL[@]}}"
fi
