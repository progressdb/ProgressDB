# ProgressDB examples: environment variables and config

This file documents the environment variables used by the ProgressDB server and the keys available in the example `config.yaml` under the `examples/` directory.

## Environment variables (examples shown in `examples/.env.example`)

- `PROGRESSDB_SERVER_ADDR` / `PROGRESSDB_SERVER_ADDRESS` / `PROGRESSDB_SERVER_PORT`: Server bind address. `PROGRESSDB_SERVER_ADDR` may be `host:port`. (Legacy `PROGRESSDB_ADDR` / `PROGRESSDB_ADDRESS` / `PROGRESSDB_PORT` accepted.)
- `PROGRESSDB_SERVER_DB_PATH`: Pebble DB path where server data is stored. (Legacy `PROGRESSDB_DB_PATH` accepted.)
- `PROGRESSDB_SERVER_CONFIG`: Optional path to a YAML config file. Flags and explicit values override config file values. (Legacy `PROGRESSDB_CONFIG` accepted.)

- `PROGRESSDB_API_BACKEND_KEYS`: Comma-separated backend API keys (server-authorized keys for backend operations).
- `PROGRESSDB_API_FRONTEND_KEYS`: Comma-separated frontend API keys (limited scope for frontend SDKs).
- `PROGRESSDB_API_ADMIN_KEYS`: Comma-separated admin API keys.

 	- `PROGRESSDB_USE_ENCRYPTION`: When `true`, the server enables encryption and requires a master key provided in the server config. Provide the master key via `security.kms.master_key_file` (recommended) or `security.kms.master_key_hex` (development).
 	- `PROGRESSDB_ENCRYPTION_FIELDS`: Comma-separated field paths to encrypt.

	- `PROGRESSDB_KMS_ENDPOINT`: Address used to connect to the KMS. Must be a TCP host:port (e.g. `127.0.0.1:6820`) or a full URL (e.g. `http://kms:6820`). Default is `127.0.0.1:6820` for external HTTP mode.
 	- `PROGRESSDB_KMS_DATA_DIR`: Directory for KMS data, wrapped DEKs, audit logs and backups.

		`PROGRESSDB_KMS_BINARY` is deprecated. The server binary includes both embedded and external KMS implementations; set `PROGRESSDB_KMS_MODE=embedded` or `PROGRESSDB_KMS_MODE=external` at runtime to choose which behavior is used.

- `PROGRESSDB_RATE_RPS` / `PROGRESSDB_RATE_BURST`: Rate limiting parameters (requests per second and burst).
- `PROGRESSDB_CORS_ORIGINS`: Comma-separated list of allowed CORS origins.
- `PROGRESSDB_IP_WHITELIST`: Optional comma-separated IP allowlist.

- `PROGRESSDB_TLS_CERT` / `PROGRESSDB_TLS_KEY`: Paths to TLS certificate and key to enable HTTPS.

## Deprecated / removed

- `PROGRESSDB_API_ALLOW_UNAUTH`: Removed. The server requires API keys for all endpoints except `GET /healthz`.
-- `PROGRESSDB_KMS_MASTER_KEY_FILE` (env): Optional. For development you may set this env var to point to a file containing the 64-hex KEK; the server prefers file-based KEK provisioning. In production prefer orchestrator secrets or a secret manager.

## Config file (`examples/config.yaml`)

- `server.address`, `server.port`: Bind address and port.
- `server.tls.cert_file`, `server.tls.key_file`: Optional TLS cert/key.
- `server.db_path`: Pebble DB path for server storage.
- `security.fields`: List of selective encryption rules (each entry: `{ path: "a.b.c", algorithm: "aes-gcm" }`).
- `security.cors.allowed_origins`: List of allowed origins.
- `security.rate_limit.rps`, `security.rate_limit.burst`: Rate limiting values.
- `security.ip_whitelist`: List of whitelisted IPs.
- `security.api_keys.backend|frontend|admin`: API key lists used by the server.
  - `security.kms.endpoint`, `security.kms.data_dir`, `security.kms.binary`: KMS integration settings (endpoint, data dir, optional binary path).
 - `encryption.use`: Boolean to enable encryption when true. This may be overridden by the environment variable `PROGRESSDB_USE_ENCRYPTION`.
 - `security.kms.master_key_hex`: Optional: embed the 64-hex (32-byte) KEK directly in the server config. Use only for controlled environments.

- `security.kms.master_key_file`: Path to a file containing the 64-hex (32-byte) KEK that the server will embed into the KMS child's config when encryption is enabled. This must be set when `PROGRESSDB_USE_ENCRYPTION=true` if `master_key_hex` is not used.
- `logging.level`: Logging level (`info`, `debug`, etc.).

		If you need the server to run with encryption enabled locally, create a `config.yaml` with either `security.kms.master_key_hex` set to a single 64-hex string (32 bytes) or `security.kms.master_key_file` pointing to a file containing the hex string, and set `PROGRESSDB_USE_ENCRYPTION=true` in your environment. For external mode start `progressdb-kms` separately and set `PROGRESSDB_KMS_MODE=external`.

If you'd like, I can also generate a minimal `examples/dev.env` and a `examples/dev-config.yaml` that are ready-to-run for local development.
