Configuration Reference
=======================

This document lists the major environment variables used by the server,
grouped by feature. Use env vars or the YAML `config.yaml` file; envs
take precedence.

Global / Server
---------------
- `PROGRESSDB_SERVER_CONFIG` — path to config file (optional).
- `PROGRESSDB_ADDR` / `PROGRESSDB_SERVER_ADDR` — HTTP listen address (host:port).
- `PROGRESSDB_DB_PATH` / `--db` flag — path to the Pebble DB directory.
- `PROGRESSDB_USE_ENCRYPTION` — enable field-level encryption (true|false).
- `PROGRESSDB_TLS_CERT`, `PROGRESSDB_TLS_KEY` — TLS cert and key paths.

Retention (automatic purge)
---------------------------
- `PROGRESSDB_RETENTION_ENABLED` (bool) — default: `false`. Enable retention autoruns.
- `PROGRESSDB_RETENTION_PERIOD` (duration) — default: `30d`. Examples: `30d`, `24h`.
- `PROGRESSDB_RETENTION_CRON` (cron string) — default: `0 2 * * *` (daily @02:00 UTC).
- `PROGRESSDB_RETENTION_BATCH_SIZE` (int) — default: `1000`. How many items to scan/delete per batch.
- `PROGRESSDB_RETENTION_BATCH_SLEEP_MS` (int) — default: `1000` (ms) pause between batches.
- `PROGRESSDB_RETENTION_DRY_RUN` (bool) — default: `false`. Log/audit but do not delete.
- `PROGRESSDB_RETENTION_PAUSED` (bool) — default: `false`. If true, runner will not start.
- `PROGRESSDB_RETENTION_MIN_PERIOD` (duration) — default: `1h`. Minimum allowed retention period.
- `PROGRESSDB_RETENTION_AUDIT_PATH` — (deprecated) previously used for audit run files. Audits now default to `<DBPath>/retention/audit.log` and the retention lock uses `<DBPath>/retention/retention.lock`.

Logging / Observability
-----------------------
- `PROGRESSDB_LOGGING_LEVEL` or `PROGRESSDB_LOG_LEVEL` — controls logger verbosity (info|debug|error).

Security / KMS
--------------
- `PROGRESSDB_KMS_ENDPOINT` — KMS service endpoint.
- `PROGRESSDB_KMS_DATA_DIR` — local KMS data directory.
- `PROGRESSDB_KMS_MASTER_KEY_FILE` / `PROGRESSDB_KMS_MASTER_KEY_HEX` — master key locator.

Notes
-----
- All duration values accept `d`, `h`, `m`, `s` where implemented (e.g. `30d`, `24h`, `15m`).
- Config precedence: flags > config file > environment variables.
- The retention subsystem validates `retention.period >= retention.min_period` at startup and fails fast if the configured period is smaller than the minimum allowed.
