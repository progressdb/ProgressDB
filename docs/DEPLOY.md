# Productionization Guide

This document lists production configuration, secrets, and operational guidance for running ProgressDB in a secure, reliable way.

It complements `server/README.md` with prescriptive defaults and a checklist for production deployments.

## Top-level goals

- Secure network access (TLS, API keys)
- Least-privilege API surface for frontend vs backend callers
- Health and liveness probes that work with orchestration platforms
- Observability (metrics, logs) and rate limiting to protect against spikes
- Encryption of persisted data when required

## Environment variables (recommended)

The server supports both a YAML `config.yaml` and environment variables. For production we recommend setting these environment variables (or their equivalents in `config.yaml`) and keeping all secret values in a secrets manager.

- `PROGRESSDB_ADDR` — Listen address (`host:port`), e.g. `0.0.0.0:8080`.
- `PROGRESSDB_DB_PATH` — Pebble DB path (persistent volume), e.g. `/var/lib/progressdb`.
- `PROGRESSDB_ENCRYPTION_KEY` — deprecated/removed: use an external KMS instead (see `server/docs/kms.md`).
- `PROGRESSDB_ENCRYPT_FIELDS` — Optional, comma-separated JSON paths to field-encrypt (e.g. `body.credit_card,body.phi.*`).
- `PROGRESSDB_CONFIG` — Optional path to `config.yaml` if you prefer file configs.
- `PROGRESSDB_LOG_LEVEL` — `debug|info|warn|error` (default `info`).

Security & API keys
- `PROGRESSDB_API_BACKEND_KEYS` — Comma-separated backend (secret) keys (scope: all routes). Use strong, random keys (e.g. `sk_...`).
- `PROGRESSDB_API_FRONTEND_KEYS` — Comma-separated frontend (public) keys (scope: limited to `GET|POST /v1/messages` and `GET /healthz`). Use distinct values (e.g. `pk_...`) and rotate independently.
- 
- `PROGRESSDB_API_ADMIN_KEYS` — Keys for admin tooling (full scope).

Networking & CORS
- `PROGRESSDB_CORS_ORIGINS` — Comma-separated allowed origins for browsers (e.g. `https://app.example.com`). If empty, no CORS headers are emitted and browsers will block cross-origin requests.
- `PROGRESSDB_IP_WHITELIST` — Comma-separated IPs allowed when IP whitelist is used (optional).

Rate limiting
- `PROGRESSDB_RATE_RPS` — Requests per second default per key/IP (e.g. `10`).
- `PROGRESSDB_RATE_BURST` — Burst size for rate limiter (e.g. `20`).

TLS
- `PROGRESSDB_TLS_CERT` — Path to TLS cert file (PEM).
- `PROGRESSDB_TLS_KEY` — Path to TLS key file (PEM).

## Equivalent `config.yaml` snippets

server:
  address: "0.0.0.0"
  port: 8080
  tls:
    cert_file: "/etc/ssl/certs/progressdb.crt"
    key_file: "/etc/ssl/private/progressdb.key"

storage:
  db_path: "/var/lib/progressdb"

security:
  # encryption is provided by an external KMS; configure under security.kms
  # e.g. security.kms.master_key_file: "/run/secrets/progressdb_kek.hex"
  cors:
    allowed_origins: ["https://app.example.com"]
  rate_limit:
    rps: 10
    burst: 20
  ip_whitelist: []
    api_keys:
      backend: ["sk_example_backend"]
      frontend: ["pk_example_frontend"]
      admin: ["admin_example"]

## CORS behavior (summary)

- If `security.cors.allowed_origins` (or `PROGRESSDB_CORS_ORIGINS`) is empty, the server does not set any CORS response headers. Browser-based cross-origin requests will be blocked by the browser.
- To allow browser clients, list specific origins. Using `*` allows all origins but is not recommended for production.

## Health checks

- The server exposes `GET /healthz` which returns `{"status":"ok"}` and is allowed without an API key by default (this is intentional to support container orchestrators). Keep this endpoint public-only and avoid returning sensitive data there.
- Use the `/metrics` endpoint for monitoring and alerting via Prometheus.

## Secrets management

- Do NOT store secrets (encryption keys, API keys) in Git. Use a secrets manager (AWS Secrets Manager, GCP Secret Manager, Vault, etc.) and inject secrets into the runtime environment or Kubernetes secrets.
- Set file permissions to owner-read/write only if you use `config.yaml` or `.env` files: `chmod 600 config.yaml .env`.

## Encryption & key rotation

`PROGRESSDB_ENCRYPTION_KEY` is deprecated/removed. Use an external KMS; see `server/docs/kms.md`.
- Rotation requires re-encrypting data; plan a migration strategy (re-encrypt on read/write or run a background re-encrypt job with a rolling window).

## Rate limiting & DOS protection

- Enable `rate_limit` in config to protect the service. Tune `rps` and `burst` based on expected traffic and per-key allowances.
- Use upstream load balancer WAF / firewall rules for coarse-grained protection.

## Observability

- Prometheus metrics at `/metrics` are enabled by default. Scrape with your Prometheus server and add alerting rules for error rates, latency, disk usage, and open file descriptors.
- Structured logs (stdout) are emitted. Ship logs to your centralized logging solution (ELK, Loki, Datadog, etc.).

## Backups & storage

- Back up the Pebble DB directory regularly. Consider filesystem snapshots or periodic copy-outs to object storage.
- Test restores to ensure backup integrity.

## Deployment checklist

- [ ] Generate a secure 32-byte encryption key: `openssl rand -hex 32` and store it in your secrets manager.
- [ ] Create backend and frontend API keys; store them securely and inject into the runtime via secrets.
- [ ] Configure TLS certs and key and verify TLS by curling the server.
- [ ] Configure allowed CORS origins for your web frontend.
- [ ] Enable rate limiting and monitoring.
- [ ] Configure health checks in your orchestrator: use `GET /healthz` for liveness and readiness probes.
- [ ] Set up Prometheus scraping on `/metrics`.
- [ ] Set filesystem permissions for any config files: `chmod 600 config.yaml`.

## Tips for rolling upgrades

- Use health checks and graceful restarts. Let your orchestrator wait for `/healthz` to be healthy before routing traffic.
- Maintain backwards compatibility for API keys — avoid rotating all keys at once.

## CI/CD & releases

- Use the provided `.goreleaser.yml` and GitHub Actions to produce release binaries and Docker images.
- Tag releases and let GoReleaser create artifacts and releases.

## Contact & support

For questions about deployment or security considerations, reach out to the engineering owner or open an issue in the repo.

## Input validation (prevent resource exhaustion)

Even with signature checks and scoped API keys, unbounded user-provided data (titles, metadata, message bodies) can lead to excessive memory, storage, or CPU use. Enforce server-side validation to allow ample content while preventing pathological payloads.

Recommended defaults (ample but bounded)
- Thread title: max 200 characters
- Thread metadata (JSON): max 8 KiB (8192 bytes) serialized
- Message body (full JSON payload): max 64 KiB (65536 bytes)
- Individual string fields inside message body (e.g., `body.text`): max 4096 characters
- Number of items in arrays (e.g., `body.tags`): max 256

Where to enforce
- At the HTTP handler boundary (before DB writes) to fail fast.
- Both structural validation (types, required fields) and size checks (bytes and per-field lengths).

Example Go validation (conceptual)
```go
func validateThreadInput(t models.Thread) error {
    if len(t.Title) > 200 {
        return fmt.Errorf("title too long")
    }
    if bs, err := json.Marshal(t.Metadata); err == nil {
        if len(bs) > 8192 { // 8 KiB
            return fmt.Errorf("metadata too large")
        }
    }
    return nil
}

func validateMessageInput(m models.Message) error {
    if bs, err := json.Marshal(m.Body); err == nil {
        if len(bs) > 65536 { // 64 KiB
            return fmt.Errorf("message body too large")
        }
    }
    if s, ok := m.Body["text"].(string); ok {
        if utf8.RuneCountInString(s) > 4096 {
            return fmt.Errorf("body.text too long")
        }
    }
    return nil
}
```

Configuration & tuning
- Make limits configurable via `config.yaml` or environment variables (e.g., `PROGRESSDB_MAX_MSG_BYTES`, `PROGRESSDB_MAX_THREAD_META_BYTES`, `PROGRESSDB_MAX_TITLE_LEN`).
- Start with conservative defaults in staging and tune after load testing.

Monitoring & alerting
- Emit metrics for validation rejections so you can detect abusive clients.
- Alert on spikes in validation failures or sudden increases in thread/message creation.

Operational notes
- For large attachments, require clients to upload to object storage and include references in messages.
- Keep validation and quotas consistent across API and admin tooling.

Rollout checklist
- [ ] Add validation checks to thread and message handlers.
- [ ] Add config knobs for limits.
- [ ] Test with large-but-valid and oversized payloads in staging.
- [ ] Monitor validation metrics and thread/message rates after rollout.

API changes: thread updates & soft-delete
---------------------------------------

- New: `PUT /v1/threads/{id}` — update thread metadata (title/attributes). Requires canonical author (verified signature for frontend; backend must supply `author` in body or via `X-User-ID`) or `admin` role.
- New: soft-delete on `DELETE /v1/threads/{id}` — marks thread `deleted=true` and appends a tombstone message. Non-admins cannot see deleted threads; admins can.
- Message `role` field: messages support an optional `role` field (e.g. `"user"`, `"system"`). The server defaults it to `"user"` when omitted.

Backend-author requirement
-------------------------

- Backend callers using a backend key must provide an `author` value when not using signature-based authentication. The server will accept `author` from the request body or the `X-User-ID` header and use it as the canonical author for the operation. Frontend callers must continue to use the signed-author flow.

Knock-on effects
-----------------
- Listings and thread message APIs hide soft-deleted threads for non-admins. Posting to or modifying messages in a soft-deleted thread is forbidden for non-admins.
- Retention: soft-deleted threads continue to consume storage; plan a separate retention/GC job to hard-delete older deleted threads.
