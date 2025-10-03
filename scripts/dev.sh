#!/usr/bin/env bash
# Ensure we run under bash (arrays and ${BASH_SOURCE} required). If invoked with
# sh, re-exec under bash so `set -u` and array usages work correctly.
if [ -z "${BASH_VERSION:-}" ]; then
  # If invoked via `sh scripts/dev.sh` then $0 == "sh" and the script path is
  # in $1. Shift so the script path becomes $0 when re-execing under bash.
  if [ "$0" = "sh" ] && [ "$#" -ge 1 ]; then
    shift
    exec bash "$@"
  else
    exec bash "$0" "$@"
  fi
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

# At this point we are running under bash.

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
  # Prefer the dedicated encryption helpers under scripts/enc/. If they are
  # missing (e.g. stripped from this checkout), provide a clear fallback or
  # error message so the dev script doesn't fail with an obscure "file not
  # found" exec error.
  if [[ "$MODE" == "embedded" ]]; then
    EMBEDDED_HELPER="$ROOT_DIR/scripts/enc/embedded/dev_enc_embedded.sh"
    if [[ -x "$EMBEDDED_HELPER" ]]; then
      exec "$EMBEDDED_HELPER" "$@"
    else
      # Fall back to the KMS dev helper which provides a development KMS
      # instance. This is not a drop-in replacement for all encryption
      # workflows but is a sensible default for local development when the
      # original enc helpers are missing.
      # Try a few likely fallback locations for the KMS dev helper. Some
      # users may invoke the script in ways that produce a doubled "scripts/"
      # path (e.g. re-exec under bash) so check common variants before erroring.
      CANDIDATES=(
        "$ROOT_DIR/scripts/kms/dev.sh"
        "$ROOT_DIR/scripts/scripts/kms/dev.sh"
        "$ROOT_DIR/kms/dev.sh"
      )
      found=0
      for cand in "${CANDIDATES[@]}"; do
        if [[ -x "$cand" || -f "$cand" ]]; then
          echo "Encryption embedded helper missing; falling back to KMS dev helper: $cand"
          exec "$cand" "$@"
          found=1
          break
        fi
      done
      if [[ $found -eq 0 ]]; then
        echo "Missing embedded encryption helper: $EMBEDDED_HELPER" >&2
        echo "Checked candidates: ${CANDIDATES[*]}" >&2
        echo "Please restore scripts/enc/embedded/dev_enc_embedded.sh or run without --enc." >&2
        exit 1
      fi
    fi
  else
    # external mode: forward wait options if set, but first check helper exists
    EXTERNAL_HELPER="$ROOT_DIR/scripts/enc/external/dev_enc_external.sh"
    if [[ ! -x "$EXTERNAL_HELPER" ]]; then
      echo "External encryption helper missing: $EXTERNAL_HELPER" >&2
      echo "Please restore scripts/enc/external/dev_enc_external.sh or run without --enc." >&2
      exit 1
    fi
    if [[ $WAIT_KMS -eq 1 ]]; then
      exec "$EXTERNAL_HELPER" --wait-kms --wait-timeout "$WAIT_TIMEOUT" "$@"
    else
      exec "$EXTERNAL_HELPER" "$@"
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
