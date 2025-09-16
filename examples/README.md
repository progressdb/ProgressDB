# ProgressDB examples: environment variables and config

This file documents the environment variables used by the ProgressDB server and the keys available in the example `config.yaml` under the `examples/` directory.

## Environment variables (examples shown in `examples/.env.example`)

- `PROGRESSDB_ADDR` / `PROGRESSDB_ADDRESS` / `PROGRESSDB_PORT`: Server bind address. `PROGRESSDB_ADDR` may be `host:port`.
- `PROGRESSDB_DB_PATH`: Pebble DB path where server data is stored.
- `PROGRESSDB_CONFIG`: Optional path to a YAML config file. Flags and explicit values override config file values.

- `PROGRESSDB_API_BACKEND_KEYS`: Comma-separated backend API keys (server-authorized keys for backend operations).
- `PROGRESSDB_API_FRONTEND_KEYS`: Comma-separated frontend API keys (limited scope for frontend SDKs).
- `PROGRESSDB_API_ADMIN_KEYS`: Comma-separated admin API keys.

	- `PROGRESSDB_USE_ENCRYPTION`: When `true`, the server requires a configured KMS and a master key provided in the server config. The server prefers a file-based master key (`security.kms.master_key_file`) when present (recommended for orchestrators); otherwise it will accept an embedded `security.kms.master_key_hex` (a 64-hex string).
- `PROGRESSDB_ENCRYPT_FIELDS`: Comma-separated field paths to encrypt.

- `PROGRESSDB_KMS_BINARY`: Optional path to a KMS binary that the server may spawn when encryption is enabled. If unset the server will search for a `kms` sibling next to the ProgressDB executable.
- `PROGRESSDB_KMS_SOCKET`: Unix-domain socket path used to connect to the KMS (default `/tmp/progressdb-kms.sock`).
- `PROGRESSDB_KMS_DATA_DIR`: Directory for KMS data, wrapped DEKs, audit logs and backups.

- `PROGRESSDB_RATE_RPS` / `PROGRESSDB_RATE_BURST`: Rate limiting parameters (requests per second and burst).
- `PROGRESSDB_CORS_ORIGINS`: Comma-separated list of allowed CORS origins.
- `PROGRESSDB_IP_WHITELIST`: Optional comma-separated IP allowlist.

- `PROGRESSDB_TLS_CERT` / `PROGRESSDB_TLS_KEY`: Paths to TLS certificate and key to enable HTTPS.

## Deprecated / removed

- `PROGRESSDB_API_ALLOW_UNAUTH`: Removed. The server requires API keys for all endpoints except `GET /healthz`.
- `PROGRESSDB_KMS_MASTER_KEY_FILE` (env): Removed. The KMS master key should be provided in the server config YAML (`security.kms.master_key_file`) when encryption is enabled.

## Config file (`examples/config.yaml`)

- `server.address`, `server.port`: Bind address and port.
- `server.tls.cert_file`, `server.tls.key_file`: Optional TLS cert/key.
- `storage.db_path`: Pebble DB path for server storage.
- `security.fields`: List of selective encryption rules (each entry: `{ path: "a.b.c", algorithm: "aes-gcm" }`).
- `security.cors.allowed_origins`: List of allowed origins.
- `security.rate_limit.rps`, `security.rate_limit.burst`: Rate limiting values.
- `security.ip_whitelist`: List of whitelisted IPs.
- `security.api_keys.backend|frontend|admin`: API key lists used by the server.
- `security.kms.socket`, `security.kms.data_dir`, `security.kms.binary`: KMS integration settings (socket, data dir, optional binary path).
- `security.kms.master_key_hex`: Optional: embed the 64-hex (32-byte) KEK directly in the server config. Use only for controlled environments.

- `security.kms.master_key_file`: Path to a file containing the 64-hex (32-byte) KEK that the server will embed into the KMS child's config when encryption is enabled. This must be set when `PROGRESSDB_USE_ENCRYPTION=true` if `master_key_hex` is not used.
- `logging.level`: Logging level (`info`, `debug`, etc.).

If you need the server to run with encryption enabled locally, create a `config.yaml` with either `security.kms.master_key_hex` set to a single 64-hex string (32 bytes) or `security.kms.master_key_file` pointing to a file containing the hex string, and set `PROGRESSDB_USE_ENCRYPTION=true` in your environment. The server will spawn or connect to the KMS and manage DEKs via the configured socket.

If you'd like, I can also generate a minimal `examples/dev.env` and a `examples/dev-config.yaml` that are ready-to-run for local development.
