#!/usr/bin/env bash
set -euo pipefail

# Provide sane defaults for development environment variables consumed
# by `scripts/dev.sh`. This file is intentionally small so it can be
# sourced from multiple working directories.

# Resolve this script's location robustly and compute project root
SCRIPT_PATH="${BASH_SOURCE[0]:-$0}"
SCRIPT_DIR="$(cd "$(dirname "$SCRIPT_PATH")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Default dev config file (can be overridden by exporting DEV_CFG)
: "${DEV_CFG:=$ROOT_DIR/scripts/config.yaml}"
export DEV_CFG

# Other dev-time defaults may be added here in future.

