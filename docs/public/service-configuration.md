---
section: service
title: "Configuration"
order: 2
visibility: public
---

# Configuration

ProgressDB can be configured via a YAML config file (`config.yaml`),
environment variables, or command-line flags. The server will also load a
`.env` file if present in the working directory.

Source precedence (highest → lowest):
- Command-line flags (`--db`, `--addr`, `--config`, etc.)
- Environment variables (e.g. `PROGRESSDB_DB_PATH`, `PROGRESSDB_ADDR`)
- YAML config file (`config.yaml`)

Common flags and env vars

- `--db` / `--db-path` / env `PROGRESSDB_DB_PATH` — Pebble DB path for storage (recommended: a persistent directory, e.g. `/var/lib/progressdb`).
- `--addr` / env `PROGRESSDB_ADDR` or `PROGRESSDB_ADDRESS`+`PROGRESSDB_PORT` — listen address and port (default `0.0.0.0:8080`).
- `--config` / env `PROGRESSDB_CONFIG` — path to YAML config file.
- TLS vars: `--tls-cert` / env `PROGRESSDB_TLS_CERT`, `--tls-key` / env `PROGRESSDB_TLS_KEY` — if set the server will enable TLS.

Important YAML config fields (examples in `docs/configs/config.yaml`)

- `server.address` / `server.port` — interface and port to listen on.
- `storage.db_path` — database storage path used by Pebble.
- `security`:
  - `cors.allowed_origins` — CORS allowed origins for browser clients.
  - `rate_limit.rps` and `rate_limit.burst` — request rate limiting.
  - `ip_whitelist` — list of IP addresses permitted to connect if set.
  - `api_keys`:
    - `backend` — backend/secret keys (e.g. `sk_...`) used to sign user IDs and call admin endpoints.
    - `frontend` — public keys (e.g. `pk_...`) for browser clients with limited scope.
    - `admin` — admin-only API keys.
- `security.kms` — KMS configuration. `mode` may be `embedded` or `external`. For production we recommend `external` and running the `progressdb-kms` daemon.
- `logging.level` — `debug|info|warn|error`.

Authentication & API keys

- All API calls require an API key. Provide it via:
  - `Authorization: Bearer <key>`
  - or `X-API-Key: <key>`
- Key scopes:
  - Backend keys (`sk_...`): full scope, can call `/v1/_sign` to sign user IDs and access admin actions.
  - Frontend keys (`pk_...`): limited scope (typically `GET|POST /v1/messages` and `/healthz`).

User signing (frontend flow)

- Backends call `POST /v1/_sign` with `{ "userId": "..." }` to obtain an HMAC-SHA256 signature bound to the provided backend key. Return the `signature` to the frontend.
- Frontend requests then include `X-User-ID` and `X-User-Signature` headers to authenticate a user identity.

KMS & encryption

- ProgressDB supports optional field-level encryption backed by a KMS.
- KMS `mode`:
  - `embedded`: in-process KMS (not recommended for production key isolation).
  - `external`: ProgressDB talks over HTTP to an external `progressdb-kms` process; this is recommended for production.
- Key & audit storage:
  - Wrapped DEKs and audit entries are persisted under the KMS data directory (see `docs/kms.md`).
  - Rotation and rewrap tooling exists in the KMS runbook.

Metrics & admin endpoints

- Health: `GET /healthz` (returns `{ "status": "ok" }`).
- Prometheus metrics: `GET /metrics`.
- Swagger UI: `GET /docs/` and OpenAPI at `GET /openapi.yaml`.
- Admin viewer (UI): available at `GET /viewer/` when the server is running locally.

Operation tips

- Always run a backup of the `storage.db_path` directory before upgrades or KMS rewraps.
- Protect API keys and KMS secrets with your secrets manager; avoid storing them in plaintext in `config.yaml` on production hosts.
- Use rate-limiting and IP whitelists for public endpoints when appropriate.

