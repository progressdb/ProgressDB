#!/usr/bin/env bash
set -euo pipefail

# Interactive publisher for the React SDK: publishes to JSR (jsr) then npm.

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
PKG_DIR="$ROOT_DIR/clients/sdk/frontend/reactjs"

YES=0
NO_BUILD=0
DRY_RUN=0
ALLOW_SLOW=0

usage(){
  cat <<EOF
Usage: $(basename "$0") [--yes] [--no-build] [--dry-run] [--allow-slow-types]

Options:
  --yes               Skip interactive prompts (assumes yes)
  --no-build          Skip building the package first
  --dry-run           For npm: run pack --dry-run instead of publish
  --allow-slow-types  Pass to jsr publish to skip slow-type blocking
  -h, --help          Show this help
EOF
}

while [[ ${1:-} != "" ]]; do
  case "$1" in
    --yes) YES=1; shift;;
    --no-build) NO_BUILD=1; shift;;
    --dry-run) DRY_RUN=1; shift;;
    --allow-slow-types) ALLOW_SLOW=1; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1"; usage; exit 1;;
  esac
done

echo "Publish React SDK: $PKG_DIR"

if [ $NO_BUILD -eq 0 ]; then
  if [ $YES -eq 0 ]; then
    read -p "Build react package first? [Y/n]: " ans
    ans=${ans:-Y}
  else
    ans=Y
  fi
  if [[ "$ans" =~ ^[Yy] ]]; then
    if ! command -v npm >/dev/null 2>&1; then
      echo "npm not found. Install Node/npm to build the package." >&2
      exit 2
    fi
    echo "Installing deps and building"
    (cd "$PKG_DIR" && npm install --no-audit --no-fund)
    (cd "$PKG_DIR" && npm run build)
  fi
fi

if ! command -v npx >/dev/null 2>&1; then
  echo "npx not found. Install Node/npm to run jsr publish." >&2
  exit 2
fi

if [ $YES -eq 0 ]; then
  read -p "Publish to JSR (jsr publish)? [Y/n]: " pubjsr
  pubjsr=${pubjsr:-Y}
else
  pubjsr=Y
fi

if [[ "$pubjsr" =~ ^[Yy] ]]; then
  JSR_ARGS=()
  if [ $ALLOW_SLOW -eq 1 ]; then
    JSR_ARGS+=("--allow-slow-types")
  fi
  echo "Running jsr publish for @progressdb/react"
  (cd "$PKG_DIR" && npx jsr publish "${JSR_ARGS[@]}")
  echo "JSR publish complete"
fi

if [ $YES -eq 0 ]; then
  read -p "Publish to npm as well? [y/N]: " pubnpm
  pubnpm=${pubnpm:-N}
else
  pubnpm=Y
fi

if [[ "$pubnpm" =~ ^[Yy] ]]; then
  (cd "$PKG_DIR")
  if [ $DRY_RUN -eq 1 ]; then
    npm pack --dry-run
    echo "npm dry-run pack complete"
    exit 0
  fi
  if ! npm whoami >/dev/null 2>&1; then
    echo "You are not logged in to npm. Run 'npm login' first." >&2
    exit 1
  fi
  npm publish --access public
  echo "npm publish complete"
fi

