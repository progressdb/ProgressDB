#!/usr/bin/env bash
set -euo pipefail

# Minimal dev script to run the server in encrypted mode using a
# repository-local developer config. This script writes
# `scripts/encrypted/config.yaml` with `security.encryption.use: true`
# and embeds a generated master key hex into the config. It ensures
# the KMS data directory and socket path are prepared, then runs the
# server with the config. No extra flags or backgrounding — the
# server runs in the foreground.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENCRYPTED_DIR="$ROOT_DIR/scripts/enc"
CFG_PATH="$ENCRYPTED_DIR/config.yaml"
SOCKET_PATH="$ENCRYPTED_DIR/progressdb-kms.sock"
DATA_DIR="$ENCRYPTED_DIR/kmsdb"
DB_PATH="$ENCRYPTED_DIR/database"

mkdir -p "$ENCRYPTED_DIR" "$DATA_DIR"

# generate a master key hex (32 bytes -> 64 hex chars)
MASTER_HEX="$(openssl rand -hex 32)"

cat > "$CFG_PATH" <<-YAML
server:
  address: "0.0.0.0"
  port: 8080
  db_path: "$DB_PATH"
  tls:
    cert_file: ""
    key_file: ""

security:
  cors:
    allowed_origins:
      - "http://localhost:3000"
      - "http://127.0.0.1:3000"
  rate_limit:
    rps: 10
    burst: 20
  ip_whitelist:
    - "127.0.0.1"
  api_keys:
    backend: ["sk_example"]
    frontend: ["pk_example"]
    admin: ["admin_example"]
  encryption:
    use: true
    fields: []
  kms:
    socket: "$SOCKET_PATH"
    data_dir: "$DATA_DIR"
    binary: ""
    master_key_file: ""
    master_key_hex: "$MASTER_HEX"

logging:
  level: "info"
YAML

echo "Wrote encrypted dev config: $CFG_PATH"

# Remove any stale socket file; the server/KMS will create a real unix socket
rm -f "$SOCKET_PATH" || true

# Ensure DB directory exists (Pebble will create directory if needed)
mkdir -p "$(dirname "$DB_PATH")"

# Run server with the encrypted config
cd "$ROOT_DIR/server"
exec go run ./cmd/progressdb --config "$CFG_PATH"
