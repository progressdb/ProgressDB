#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/../.." && pwd)
PKG_DIR="$ROOT_DIR/clients/sdk/backend/python"

echo "Running Python SDK tests in $PKG_DIR"
cd "$PKG_DIR"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 2
fi

python3 -m pip install --upgrade pip
python3 -m pip install pytest responses

python3 -m pytest -q

