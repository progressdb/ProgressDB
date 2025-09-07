#!/usr/bin/env bash
set -euo pipefail

# Build and publish Python SDK to PyPI (requires twine and credentials)
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
PKG_DIR="$ROOT_DIR/clients/sdk/backend/python"

cd "$PKG_DIR"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found" >&2
  exit 2
fi

if ! command -v pip >/dev/null 2>&1; then
  echo "pip not found" >&2
  exit 2
fi

python3 -m pip install --upgrade build twine
python3 -m build

if [ "${PYPI_DRY_RUN:-0}" = "1" ]; then
  echo "Dry run mode, not uploading"
  exit 0
fi

if ! command -v twine >/dev/null 2>&1; then
  echo "twine not found" >&2
  exit 2
fi

echo "Uploading to PyPI (will prompt for credentials if not set via env)"
twine upload dist/*

