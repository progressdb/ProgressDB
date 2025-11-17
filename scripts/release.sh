#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

printf "Component (db-service, js-sdk, node-sdk, py-sdk, react-sdk): "
read -r COMPONENT

printf "Version: "
read -r VERSION

CLEAN_VERSION="${VERSION#v}"

case "$COMPONENT" in
  db-service) TAG_NAME="service-v${CLEAN_VERSION}" ;;
  js-sdk) TAG_NAME="sdk-js-v${CLEAN_VERSION}" ;;
  node-sdk) TAG_NAME="sdk-node-v${CLEAN_VERSION}" ;;
  py-sdk) TAG_NAME="sdk-py-v${CLEAN_VERSION}" ;;
  react-sdk) TAG_NAME="sdk-react-v${CLEAN_VERSION}" ;;
  *) echo "Invalid component" >&2; exit 1 ;;
esac

echo "Creating release tag: $TAG_NAME"

git diff-index --quiet HEAD -- || { echo "Uncommitted changes" >&2; exit 1; }
git rev-parse "$TAG_NAME" >/dev/null 2>&1 && git tag -d "$TAG_NAME"
git ls-remote --tags origin "$TAG_NAME" | grep -q "$TAG_NAME" && git push origin ":refs/tags/$TAG_NAME"

git tag -a "$TAG_NAME" -m "ProgressDB $CLEAN_VERSION"
git push origin "$TAG_NAME"

echo "Release triggered for $TAG_NAME"