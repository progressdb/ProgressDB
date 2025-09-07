#!/bin/sh
set -eu

# Publish the Node backend SDK to JSR
# Usage: .scripts/publish-jsr.sh [--dry-run]
#   --dry-run    Preview publish only

DRY_RUN="false"

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run)
      DRY_RUN="true"; shift ;;
    *)
      echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

if ! command -v deno >/dev/null 2>&1; then
  echo "Deno CLI not found. Install from https://deno.land/#installation" >&2
  exit 1
fi

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
SDK_DIR=$(cd "$SCRIPT_DIR/../clients/sdk/backend/nodejs" && pwd)

cd "$SDK_DIR"

if [ ! -f jsr.json ]; then
  echo "jsr.json not found in $SDK_DIR" >&2
  exit 1
fi

ARGS=""
if [ "$DRY_RUN" = "true" ]; then
  ARGS="$ARGS --dry-run"
fi
ARGS="$ARGS --unstable-sloppy-imports"

echo "Publishing to JSR from $SDK_DIR ..."
# shellcheck disable=SC2086
# shellcheck disable=SC2086
deno publish $ARGS
echo "JSR publish completed."
