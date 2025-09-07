#!/bin/sh
set -eu

# Publish the Node backend SDK to npm
# Usage: .scripts/publish-npm.sh [--tag <tag>] [--dry-run] [--no-build] [--install]

TAG=""
DRY_RUN="false"
NO_BUILD="false"
DO_INSTALL="false"

while [ $# -gt 0 ]; do
  case "$1" in
    --tag)
      TAG="$2"; shift 2 ;;
    --dry-run)
      DRY_RUN="true"; shift ;;
    --no-build)
      NO_BUILD="true"; shift ;;
    --install)
      DO_INSTALL="true"; shift ;;
    *)
      echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
SDK_DIR=$(cd "$SCRIPT_DIR/../clients/sdk/backend/nodejs" && pwd)

cd "$SDK_DIR"

echo "Checking npm auth..."
if ! npm whoami >/dev/null 2>&1; then
  echo "Not logged into npm. Run: npm login" >&2
  exit 1
fi

if [ "$NO_BUILD" != "true" ]; then
  if [ "$DO_INSTALL" = "true" ]; then
    echo "Installing deps with npm ci..."
    npm ci
  fi
  echo "Building SDK..."
  npm run build
fi

ARGS="--access public"
if [ -n "$TAG" ]; then
  ARGS="$ARGS --tag $TAG"
fi
if [ "$DRY_RUN" = "true" ]; then
  ARGS="$ARGS --dry-run"
fi

echo "Publishing to npm from $SDK_DIR ..."
# shellcheck disable=SC2086
npm publish $ARGS
echo "npm publish completed."

