---
section: service
title: "Configuration"
order: 2
visibility: public
---

# Configuration

ProgressDB supports three configuration sources (highest → lowest precedence):

- Command-line flags (e.g. `--db`, `--addr`, `--config`)
- Environment variables (e.g. `PROGRESSDB_DB_PATH`, `PROGRESSDB_ADDR`)
- YAML config file (default `config.yaml`)

Below is a comprehensive example `config.yaml` that shows the supported
options. After the example there is a short explanation of each option and
the equivalent environment variable (when available).

Example `config.yaml` (complete)

```yaml
server:
  address: "0.0.0.0"
  port: 8080
  db_path: "./data"
  tls:
    cert_file: ""      # path to TLS cert file (enable TLS when set)
    key_file: ""       # path to TLS key file

storage:
  db_path: "./data/store"   # Pebble DB directory

security:
  cors:
    allowed_origins:
      - "http://localhost:3000"
      - "https://example.com"
  rate_limit:
    rps: 10
    burst: 20
  ip_whitelist: []
  api_keys:
    backend: ["sk_example"]
    frontend: ["pk_example"]
    admin: ["admin_example"]
  encryption:
    use: false
    fields: []
  kms:
    mode: "external"     # embedded | external
    endpoint: "127.0.0.1:6820"
    data_dir: "./kms-data"
    binary: "/usr/local/bin/progressdb-kms"
    master_key_file: ""   # embedded mode only
    master_key_hex: ""    # alternative to file (embedded only)

logging:
  level: "info"   # debug|info|warn|error

validation:
  required: []
  types: []
  max_len: []
  enums: []
  when_then: []

retention:
  enabled: false
  days: 0

metrics:
  enabled: true
  path: "/metrics"

admin:
  enable_viewer: true
  viewer_path: "/viewer/"
```

Configuration reference (option → explanation → env var)

- `server.address`, `server.port`
  - What it does: network interface and port for the HTTP server. Use `0.0.0.0` to listen on all interfaces.
  - Env vars: `PROGRESSDB_ADDRESS` and `PROGRESSDB_PORT` (or `PROGRESSDB_ADDR` as a combined `host:port`).

- `server.db_path` / `storage.db_path`
  - What it does: path to the Pebble DB files. Must be persistent and writable by the server process.
  - Env var: `PROGRESSDB_DB_PATH`.

- `server.tls.cert_file`, `server.tls.key_file`
  - What it does: when both are set the server enables TLS. Provide full filesystem paths to the cert and key.
  - Env vars: `PROGRESSDB_TLS_CERT`, `PROGRESSDB_TLS_KEY`.

- `security.cors.allowed_origins`
  - What it does: list of allowed origins for browser CORS. Wildcards are not recommended in production.
  - Env var: `PROGRESSDB_CORS_ORIGINS` (comma-separated).

- `security.rate_limit.rps`, `security.rate_limit.burst`
  - What it does: enables per-key or per-IP rate limiting (requests per second and burst).
  - Env vars: `PROGRESSDB_RATE_RPS`, `PROGRESSDB_RATE_BURST`.

- `security.ip_whitelist`
  - What it does: if non-empty, only requests from listed IPs are permitted.

- `security.api_keys.backend`, `security.api_keys.frontend`, `security.api_keys.admin`
  - What it does: lists of API keys by scope. Backend keys (`sk_...`) are privileged and may call `/v1/_sign` and admin routes. Frontend keys (`pk_...`) are limited (typically to message endpoints and health).
  - Env vars: `PROGRESSDB_API_BACKEND_KEYS`, `PROGRESSDB_API_FRONTEND_KEYS`, `PROGRESSDB_API_ADMIN_KEYS` (comma-separated).

- `security.encryption.use`, `security.encryption.fields`
  - What it does: enable field-level encryption and list JSON paths to encrypt (e.g., `body.credit_card`). When `use: true` the server will attempt decryption on reads.
  - Note: full-message encryption vs field-level: configuration defines behavior; see `server/docs/encryption.md`.

- `security.kms.mode` (embedded|external)
  - What it does: selects the KMS provider mode. `embedded` runs an in-process KMS (dev/test). `external` makes HTTP calls to a separate `progressdb-kms` service (recommended for production).
  - Env var: `PROGRESSDB_KMS_MODE`.

- `security.kms.endpoint`
  - What it does: network address (host:port or URL) of the external KMS service.
  - Env var: `PROGRESSDB_KMS_ENDPOINT`.

- `security.kms.data_dir`, `security.kms.binary`, `master_key_file`, `master_key_hex`
  - What they do: KMS runtime and storage options. `data_dir` is where KMS metadata and wrapped keys are stored. `binary` is the external KMS executable path used in some deployments. `master_key_file` / `master_key_hex` are for embedded KMS master key provisioning only.

- `logging.level`
  - What it does: logging verbosity. Use `info` for normal ops and `debug` for troubleshooting.
  - Env var: `PROGRESSDB_LOG_LEVEL`.

- `validation` (required, types, max_len, enums, when_then)
  - What it does: optional JSON path validation rules applied at write time (server accepts flexible JSON body but can enforce constraints here).

- `retention.enabled`, `retention.days`
  - What it does: if retention is enabled the server may periodically garbage collect old messages per policy. See `server/docs/retention.md` for specifics.

- `metrics.enabled`, `metrics.path`
  - What it does: enables Prometheus metrics endpoint (default `/metrics`).

- `admin.enable_viewer`, `admin.viewer_path`
  - What it does: enables the local admin viewer UI and its URL path (useful for local debugging; restrict in production).

Command-line flags (common)

- `--db` or `--db-path` — shorthand for `storage.db_path` / `server.db_path`.
- `--addr` — address to bind (host:port). Overrides `server.address`/`server.port`.
- `--config` — path to a YAML config file.
- `--tls-cert` / `--tls-key` — enable TLS using provided files.

Notes & best practices

- Do not store long-lived backend API keys in plaintext in `config.yaml` on production hosts — use a secrets manager and inject keys via environment variables or your orchestration secrets mechanism.
- Prefer `security.kms.mode: external` in production and run `progressdb-kms` on a separate host with strict access controls.
- Always snapshot the DB path before performing upgrades or KMS rewrap operations.

