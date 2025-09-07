#!/usr/bin/env bash
set -euo pipefail

# Publish the Python backend SDK (clients/sdk/backend/python)
# Wrapper placed in /.scripts/sdk to standardize SDK publishes.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
PKG_DIR="$ROOT_DIR/clients/sdk/backend/python"

YES=0
DRY_RUN=0

usage(){
  cat <<EOF
Usage: $(basename "$0") [--yes] [--dry-run]

Options:
  --yes       Skip prompts and proceed
  --dry-run   Build only (do not upload)
  -h, --help  Show this help
EOF
}

while [[ ${1:-} != "" ]]; do
  case "$1" in
    --yes) YES=1; shift;;
    --dry-run) DRY_RUN=1; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1"; usage; exit 1;;
  esac
done

# if [ "$EUID" -eq 0 ]; then
#   echo "Do not run this script as root/sudo. Re-run without sudo." >&2
#   exit 1
# fi

echo "Publishing Python SDK in $PKG_DIR"
cd "$PKG_DIR"

if [ $YES -eq 0 ]; then
  read -p "Build and publish Python package to PyPI? [y/N]: " ans
  ans=${ans:-N}
else
  ans=Y
fi

if [[ ! "$ans" =~ ^[Yy] ]]; then
  echo "Aborted by user"
  exit 0
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found. Install Python 3.8+ to build the package." >&2
  exit 2
fi

python3 -m pip install --upgrade build twine
python3 -m build

if [ $DRY_RUN -eq 1 ]; then
  echo "Dry run mode: built artifacts in $PKG_DIR/dist but not uploading."
  exit 0
fi

echo "Uploading to PyPI using twine (interactive). Set TWINE_USERNAME/TWINE_PASSWORD or use keyring as needed."
python3 -m twine upload dist/*

echo "Publish complete."

