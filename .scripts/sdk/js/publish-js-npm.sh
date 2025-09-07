#!/usr/bin/env bash
set -euo pipefail

# Publish the compiled SDK to npm. Ensure dist is built and you are logged in.
ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SDK_DIR="$ROOT_DIR/clients/sdk/frontend/typescript"

cd "$SDK_DIR"

if [ ! -d dist ]; then
  echo "dist not found — run build first: scripts/sdk/js/build-js-sdk.sh" >&2
  exit 1
fi

echo "Publishing @progrssdb/js to npm"
echo "Make sure you are logged in (npm login) and have permissions for the package scope."

npm publish --access public dist || {
  echo "npm publish failed" >&2
  exit 1
}

echo "Published"

