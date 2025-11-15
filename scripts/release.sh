#!/usr/bin/env bash
if [ -z "${BASH_VERSION:-}" ]; then
  if [ "$0" = "sh" ] && [ "$#" -ge 1 ]; then
    shift
    exec bash "$@"
  else
    exec bash "$0" "$@"
  fi
fi
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

usage() {
  echo "Usage: $0 <version>" >&2
  echo "  version    Version string (e.g., 0.2.0-alpha, v0.2.0-alpha)" >&2
  exit 1
}

if [ $# -eq 0 ]; then
  echo -n "Enter version (e.g., 0.2.0-alpha): "
  read -r VERSION
  if [ -z "$VERSION" ]; then
    echo "Error: Version is required" >&2
    exit 1
  fi
else
  VERSION="$1"
fi

if [[ ! "$VERSION" =~ ^service-v ]]; then
  if [[ "$VERSION" =~ ^v ]]; then
    TAG_NAME="service-${VERSION}"
  else
    TAG_NAME="service-v${VERSION}"
  fi
else
  TAG_NAME="$VERSION"
fi

CLEAN_VERSION="${TAG_NAME#service-v}"

echo "Creating release tag: $TAG_NAME"

if ! git diff-index --quiet HEAD --; then
  echo "Error: Working directory has uncommitted changes" >&2
  exit 1
fi

if git rev-parse "$TAG_NAME" >/dev/null 2>&1; then
  git tag -d "$TAG_NAME"
fi

if git ls-remote --tags origin "$TAG_NAME" | grep -q "$TAG_NAME"; then
  git push origin ":refs/tags/$TAG_NAME"
fi

git tag -a "$TAG_NAME" -m "ProgressDB $CLEAN_VERSION"
git push origin "$TAG_NAME"

echo "✅ Release triggered for $TAG_NAME"