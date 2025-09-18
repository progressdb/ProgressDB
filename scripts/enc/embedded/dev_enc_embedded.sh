#!/usr/bin/env bash
set -euo pipefail

# Embedded-mode encrypted dev runner (per-mode folder)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
TEMPLATE="$SCRIPT_DIR/config.template.yaml"
OUT_CFG="$SCRIPT_DIR/config.generated.yaml"

if [[ ! -f "$TEMPLATE" ]]; then
  echo "Missing template: $TEMPLATE" >&2; exit 1
fi

MASTER_HEX="$(openssl rand -hex 32)"

# Replace placeholder master_key_hex in template -> generated config
awk -v mk="$MASTER_HEX" '/master_key_hex:/ { sub(/master_key_hex: ".*"/, "master_key_hex: \"" mk "\"") } { print }' "$TEMPLATE" > "$OUT_CFG"

export PROGRESSDB_USE_ENCRYPTION=1
export PROGRESSDB_KMS_MODE=embedded

echo "DEV (enc:embedded): using config $OUT_CFG"

cd "$ROOT_DIR/server"
mkdir -p .gopath/pkg/mod
export GOPATH="$PWD/.gopath"
export GOMODCACHE="$PWD/.gopath/pkg/mod"

exec go run ./cmd/progressdb --config "$OUT_CFG" "$@"

