# Server — In-Depth Developer Guide

This document explains the current ProgressDB server architecture, runtime components, middleware chain, configuration, logging, KMS integration, storage model, APIs, and developer guidance for testing and debugging.

1) High-level architecture
--------------------------
- The server is a Go HTTP service exposing a versioned API under `/v1` and admin routes under `/admin`.
- Core responsibilities:
  - HTTP API surface (messages, threads, admin)
  - Persistence (Pebble key-value store)
  - KMS integration (encryption services via provider bridge)
  - Authentication & authorization middleware
  - Observability (structured logging and metrics)

2) Major packages & responsibilities
------------------------------------
- `server/cmd/progressdb` — CLI entry; loads config, initializes logger, constructs App, runs lifecycle (graceful shutdown).
- `server/internal/app` — application bootstrap, KMS setup, HTTP server start/stop orchestration.
- `server/pkg/api` — HTTP router and handler registration. Subpackages:
  - `handlers` — implement endpoints for messages, threads, admin, signing.
- `server/pkg/store` — Pebble-backed storage helpers (messages, threads, keys, indexes).
- `server/pkg/security` — encryption abstraction and KMS provider bridge.
- `server/pkg/kms` — provider adapters (remote client and embedded adapters).
- `server/pkg/auth` — signature verification and author resolution.
- `server/pkg/logger` — centralized Zap logger and request logging helpers.

3) Configuration
----------------
- Sources: flags (`--config` etc.), config file (`server/scripts/config.yaml` examples), environment variables.
- Important runtime flags/envs:
  - `PROGRESSDB_LOG_MODE` (dev|prod)
  - `PROGRESSDB_KMS_ENDPOINT` (for external KMS)
  - Server config YAML: defines server.port, server.db_path, security settings, KMS settings, api keys.
- Config is parsed into an effective config at startup and validated; startup fails fast on invalid config (e.g., encryption enabled but no master key provided).

4) HTTP & middleware chain
--------------------------
- Router: `github.com/gorilla/mux`.
- Middleware ordering (registered in `server/pkg/api/http.go`):
  1. Security middleware (`AuthenticateRequestMiddleware`)
     - CORS preflight, rate limiting, IP whitelist, API key parsing, role resolution, per-request logging.
  2. Signature middleware (`RequireSignedAuthor`) applied to protected subrouter
     - Verifies HMAC signatures and injects canonical author into request context.
  3. Handlers: messages, threads, admin. Handlers call `auth.ResolveAuthorFromRequest` to determine canonical author and enforce ownership.

5) Persistence model (Pebble)
----------------------------
- Messages:
  - Key: `thread:<threadID>:msg:<timestamp>-<seq>` storing ciphertext or plaintext JSON.
  - Version index: `version:msg:<msgID>:<ts>-<seq>` stores per-version values.
- Threads:
  - Key: `thread:<threadID>:meta` storing JSON metadata including `kms` section.
- Keys/metadata:
  - Wrapped DEKs and other small items stored via `store.SaveKey` under chosen namespaces.

6) Encryption & KMS integration
--------------------------------
- Thread-first DEK provisioning: createThread provisions a per-thread DEK via provider and embeds `kms.key_id` into thread meta.
 - Message writes read thread meta `kms.key_id` and call `security.EncryptWithDEK(keyID, plaintext)`.
 - Message reads use `security.DecryptWithDEK(keyID, ciphertext)`.
- Provider abstraction supports both remote KMS and embedded provider.

7) Authentication & authorization (summary)
-------------------------------------------
- API keys (backend/admin) are defined in config and used to resolve role.
- Signature-based flow (frontend) uses `X-User-ID` + `X-User-Signature` HMAC-SHA256.
- Middleware ordering ensures security checks first, then signature verification, then handler-level ownership checks.

8) Logging & observability
--------------------------
- Centralized `server/pkg/logger` using Zap. Configure via env:
  - `PROGRESSDB_LOG_MODE` (dev|prod), `PROGRESSDB_LOG_SINK`, `PROGRESSDB_LOG_LEVEL`.
- Request logging: compact headers and single-line structured entries.
- Metrics: server exports Prometheus metrics (package imports `prometheus/client_golang`); instrument handlers and internal stats as needed.

9) Admin & maintenance endpoints
--------------------------------
- Admin endpoints include `/admin/health`, `/admin/stats`, `/admin/threads`, key management and DEK rotation endpoints.
- Admin routes require admin role (AdminApiKey security) and are rate-limited and audited.

10) Testing & development
-------------------------
- Embedded provider mode is recommended for unit/integration tests.
- Dev scripts:
  - `scripts/dev.sh` with `--enc` toggles and `scripts/enc/*` helpers for encrypted dev configs.
- Recommended test flows:
  - create thread → ensure `thread.meta.kms.key_id` exists → post an encrypted message → fetch and verify plaintext.

11) Troubleshooting & common issues
-----------------------------------
- Decryption failures: verify thread meta `kms.key_id`, KMS health, and provider compatibility (nonce/ciphertext shape).
- Missing DEK mapping: ensure threads are created before messages when encryption is enabled.
- Permission issues when building: some environments may restrict local module cache; set `GOCACHE` and `GOMODCACHE` to local writable directories for CI/dev runs.

12) Code pointers (where to look)
---------------------------------
- CLI + bootstrap: `server/cmd/progressdb/main.go`
- App lifecycle: `server/internal/app`
- HTTP router: `server/pkg/api/http.go`
- Handlers: `server/pkg/api/handlers/*.go`
- Store helpers: `server/pkg/store/pebble.go`
- Security/KMS bridge: `server/pkg/security/crypto.go`, `server/pkg/kms/*`
- Auth: `server/pkg/auth/*`
- Logger: `server/pkg/logger/*`
