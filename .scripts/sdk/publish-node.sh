#!/usr/bin/env bash
set -euo pipefail

# Interactive single-file publisher for npm (Node)
# Prompts to optionally bump version, builds, runs npm pack (dry-run) and npm publish

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SDK_DIR="$ROOT_DIR/clients/sdk/frontend/typescript"

yes_flag=0
build_first=1
dry_run=0
allow_slow=0

usage(){
  cat <<EOF
Usage: $(basename "$0") [--yes] [--no-build] [--dry-run]

Options:
  --yes        Skip prompts and proceed with defaults
  --no-build   Skip the build step
  --dry-run    Run npm pack --dry-run instead of publishing
  -h, --help   Show this help
EOF
}

while [[ ${1:-} != "" ]]; do
  case "$1" in
    --yes) yes_flag=1; shift;;
    --no-build) build_first=0; shift;;
    --dry-run) dry_run=1; shift;;
    --allow-slow-types) allow_slow=1; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1"; usage; exit 1;;
  esac
done

echo "Publisher — will publish to JSR (jsr) first, then npm. SDK dir: $SDK_DIR"

if [ $build_first -eq 1 ]; then
  if [ $yes_flag -eq 0 ]; then
    read -p "Build the SDK first? [Y/n]: " ans
    ans=${ans:-Y}
  else
    ans=Y
  fi
  if [[ "$ans" =~ ^[Yy] ]]; then
    # Inline build logic
    if ! command -v npm >/dev/null 2>&1; then
      echo "npm not found. Install Node/npm to build the SDK." >&2
      exit 2
    fi
    echo "Installing dependencies (npm install)"
    (cd "$SDK_DIR" && npm install --no-audit --no-fund)
    echo "Running npm run build"
    (cd "$SDK_DIR" && npm run build)
    echo "Build complete. Output in $SDK_DIR/dist"
  fi
fi

cd "$SDK_DIR"

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found. Install Node/npm to publish." >&2
  exit 2
fi

PKG_NAME=$(node -e "console.log(require('./package.json').name)")
PKG_VER=$(node -e "console.log(require('./package.json').version)")

echo "Package: $PKG_NAME@$PKG_VER"

if [ $yes_flag -eq 0 ]; then
  read -p "Bump version? (patch/minor/major/none) [patch]: " bump
  bump=${bump:-patch}
else
  bump=none
fi

if [ "$bump" != "none" ]; then
  # Use node to bump semver precisely
  node -e '
const fs=require("fs");const p=JSON.parse(fs.readFileSync("package.json","utf8"));
const sem=p.version.split(".").map(Number);
if(process.argv[1]==="patch")sem[2]++;
else if(process.argv[1]==="minor"){sem[1]++;sem[2]=0}
else if(process.argv[1]==="major"){sem[0]++;sem[1]=0;sem[2]=0}
else if(process.argv[1]!="none"){console.error("unknown bump");process.exit(2)}
p.version=sem.join(".");fs.writeFileSync("package.json",JSON.stringify(p,null,2)+"\n");console.log("bumped to",p.version);
' "$bump"
  PKG_VER=$(node -e "console.log(require('./package.json').version)")
  echo "New version: $PKG_VER"
fi


# First: publish to JSR (Deno) registry
if [ $yes_flag -eq 0 ]; then
  read -p "Publish to JSR (jsr.io) first? [Y/n]: " jsr_ans
  jsr_ans=${jsr_ans:-Y}
else
  jsr_ans=Y
fi

if [[ "$jsr_ans" =~ ^[Yy] ]]; then
  if ! command -v npx >/dev/null 2>&1; then
    echo "npx not found. Install Node/npm to run jsr publish." >&2
    exit 2
  fi
  if [ ! -f "$SDK_DIR/mod.ts" ]; then
    echo "mod.ts not found in $SDK_DIR. Ensure mod.ts exists and package.json exports it." >&2
    exit 1
  fi
  JSR_ARGS=()
  if [ $allow_slow -eq 1 ]; then
    JSR_ARGS+=("--allow-slow-types")
  fi
  echo "Running jsr publish ${JSR_ARGS[*]}"
  npx jsr publish "${JSR_ARGS[@]}"
  echo "jsr publish complete"
else
  echo "Skipping jsr publish as requested"
fi

if ! npm whoami >/dev/null 2>&1; then
  echo "You are not logged in to npm. Run 'npm login' first." >&2
  exit 1
fi

if [ $dry_run -eq 1 ]; then
  echo "Running npm pack --dry-run"
  npm pack --dry-run
  echo "Dry run complete. No publish performed."
  exit 0
fi

echo "Publishing $PKG_NAME@$PKG_VER to npm"
npm publish --access public
echo "Published $PKG_NAME@$PKG_VER"

# After npm publish we are done. But per workflow, we should publish to jsr first.
