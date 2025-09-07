#!/usr/bin/env bash
set -euo pipefail

# This script interactively publishes the SDK to the JSR (Deno) registry.
# It prompts for build, optionally allows slow type checking, and runs npx jsr publish.
# After a successful JSR publish, it offers to publish to npm as well.

# Set the root directory and SDK directory
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SDK_DIR="$ROOT_DIR/clients/sdk/frontend/typescript"

# yes_flag: if set to 1, skip prompts and use defaults
yes_flag=0
# allow_slow: if set to 1, pass --allow-slow-types to jsr publish
allow_slow=0
# build_first: if set to 1, perform the build step before publishing
build_first=1

# publish_npm: always 1, controls whether to offer npm publish after jsr publish
publish_npm=1

# usage: prints help message for script usage
usage(){
  cat <<EOF
Usage: $(basename "$0") [--yes] [--no-build] [--allow-slow-types]

Options:
  --yes             Skip prompts and proceed with defaults (build + publish)
  --no-build        Skip the build step
  --allow-slow-types Pass --allow-slow-types to jsr publish
  -h, --help        Show this help
EOF
}

# Parse command line arguments
while [[ ${1:-} != "" ]]; do
  case "$1" in
    --yes) yes_flag=1; shift;;
    --no-build) build_first=0; shift;;
    --allow-slow-types) allow_slow=1; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1"; usage; exit 1;;
  esac
done

echo "JSR Publisher — SDK dir: $SDK_DIR"

# Optionally build the SDK before publishing
if [ $build_first -eq 1 ]; then
  if [ $yes_flag -eq 0 ]; then
    read -p "Build the SDK first? [Y/n]: " ans
    ans=${ans:-Y}
  else
    ans=Y
  fi
  if [[ "$ans" =~ ^[Yy] ]]; then
    # Check for npm and build the SDK
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

# Check that mod.ts exists in the SDK directory
if [ ! -f "$SDK_DIR/mod.ts" ]; then
  echo "Error: mod.ts not found in $SDK_DIR. Ensure mod.ts exists and package.json exports it." >&2
  exit 1
fi

# Check for npx, required for jsr publish
if ! command -v npx >/dev/null 2>&1; then
  echo "npx not found. Install Node/npm to run jsr publish." >&2
  exit 2
fi

# Prepare arguments for jsr publish
JSR_ARGS=()
if [ $allow_slow -eq 1 ]; then
  JSR_ARGS+=("--allow-slow-types")
fi

# Confirm with user before running jsr publish, unless --yes is set
if [ $yes_flag -eq 0 ]; then
  echo "About to run: npx jsr publish ${JSR_ARGS[*]}"
  read -p "Proceed with jsr publish? [y/N]: " proceed
  proceed=${proceed:-N}
else
  proceed=Y
fi

if [[ "$proceed" =~ ^[Yy] ]]; then
  # Run jsr publish with the prepared arguments
  npx jsr publish "${JSR_ARGS[@]}"
  echo "jsr publish completed"

  # Offer to publish to npm after jsr publish
  if [ $yes_flag -eq 0 ]; then
    read -p "Also publish the SDK to npm? [y/N]: " npm_proceed
    npm_proceed=${npm_proceed:-N}
  else
    npm_proceed=Y
  fi

  if [[ "$npm_proceed" =~ ^[Yy] ]]; then
    # Check for npm before proceeding
    if ! command -v npm >/dev/null 2>&1; then
      echo "npm not found. Install Node/npm to publish to npm." >&2
      exit 2
    fi

    # Change to SDK directory for npm operations
    cd "$SDK_DIR"

    # Optionally bump the npm package version
    if [ $yes_flag -eq 0 ]; then
      read -p "Bump npm package version? (patch/minor/major/none) [patch]: " bump
      bump=${bump:-patch}
    else
      bump=none
    fi

    # If bump is not "none", bump the version in package.json
    if [ "$bump" != "none" ]; then
      node -e '\nconst fs=require("fs");const p=JSON.parse(fs.readFileSync("package.json","utf8"));\nconst sem=p.version.split(".").map(Number);\nif(process.argv[1]==="patch")sem[2]++;\nelse if(process.argv[1]==="minor"){sem[1]++;sem[2]=0}\nelse if(process.argv[1]==="major"){sem[0]++;sem[1]=0;sem[2]=0}\nelse if(process.argv[1]!="none"){console.error("unknown bump");process.exit(2)}\np.version=sem.join(".");fs.writeFileSync("package.json",JSON.stringify(p,null,2)+"\\n");console.log("bumped to",p.version);\n' "$bump"
    fi

    # Ensure dependencies are installed and build artifacts exist before npm publish
    echo "Installing deps and ensuring build before npm publish"
    npm install --no-audit --no-fund
    npm run build

    # Check if user is logged in to npm
    if ! npm whoami >/dev/null 2>&1; then
      echo "You are not logged in to npm. Run 'npm login' first." >&2
      exit 1
    fi

    # Publish to npm
    echo "Publishing to npm"
    npm publish --access public
    echo "npm publish completed"
  else
    echo "Skipping npm publish as requested"
  fi

else
  echo "Aborted by user"
fi
