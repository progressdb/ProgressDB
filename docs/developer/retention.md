# Retention Design & Operational Guidance

This document explains the decision logic and recommended patterns for implementing
the retention feature (permanent purge of soft-deleted content) in the server.
It collects rationale, configuration options, operational controls, and tuning
guidance so engineers can implement a robust, production-safe retention worker.

--

Contents
- Purpose
- High-level approaches (and when each is acceptable)
- Recommended approach and rationale
- Configuration & environment variables
- Batch sizing and tuning guidance
- Safety, idempotency and transactional considerations
- Observability and operator controls
- Testing guidance
- Implementation checklist

--

Purpose
-------

Retention permanently removes soft-deleted content after a configurable delay.
The goal is to provide predictable, safe cleanup that avoids impacting normal
traffic and keeps data stores consistent (threads, messages, versions, reactions).

High-level approaches
---------------------

1) Simple single-pass (get-all → delete one-by-one)
   - Behavior: query all matching items and delete them sequentially.
   - When acceptable: very small datasets, maintenance windows, or one-off migrations.
   - Problems at scale: long-running runs, timeouts, large memory usage if not streamed,
     high DB load, poor failure recovery and no clear progress checkpointing.

2) Batching (recommended default)
   - Behavior: query up to `BATCH_SIZE` candidates (ordered deterministically), delete them
     in a short transaction or in per-item transactions, then loop until none remain.
   - Benefits: bounded work per loop, predictable resource usage, easy retries, metrics per batch.

3) Streaming with checkpoints
   - Behavior: stream candidate IDs using a cursor or indexed pagination and delete in small
     transactions, storing a checkpoint to resume on failure.
   - Benefits: scales to very large datasets; requires checkpoint bookkeeping.

4) DB-native TTL / partition drop
   - Behavior: leverage database features (TTL, partitioning + DROP PARTITION) to expire data.
   - Benefits: efficient and fast; limited portability and may require schema changes.

5) Queue-driven per-item deletes
   - Behavior: enqueue IDs to delete and let a pool of workers process them at controlled rate.
   - Benefits: flexible orchestration across stores; higher operational complexity.

Why not always delete everything one-by-one?
-------------------------------------------

Deleting everything sequentially can be acceptable for small systems, but at production
scale it is risky because:
- It can run for a very long time and be interrupted.
- It may create large transactions or many small transactions that collectively
  overload the DB and cause replication lag.
- Partial failures make it hard to know where to resume; batching provides natural
  checkpoints and smaller rollback scopes.

Recommended approach
--------------------

Start with a batching worker that:

- Calculates a cutoff timestamp: `cutoff = now - retentionPeriod`.
- Repeats:
  1. Query for up to `BATCH_SIZE` soft-deleted items with `deleted_at <= cutoff` ordered by `deleted_at, id`.
  2. For each item in the batch, perform a safe purge (preferably in a short DB transaction):
     - Delete versions/reactions/related data first (or rely on FK cascades in the same transaction).
     - Delete the primary record.
  3. Record counts and errors, optionally pause briefly between batches.
  4. Continue until the query returns no items.

Rationale: batching gives operational control, predictable resource usage, and easier
failure handling while still completing cleanup in a reasonable time.

Configuration & environment variables
------------------------------------

Use configuration precedence: environment variables override config file values, which
override defaults.

Suggested environment variables (names and defaults):

- `PROGRESSDB_RETENTION_ENABLED` (boolean, default: `true`)
  - Globally enable/disable the retention worker.

- `PROGRESSDB_RETENTION_PERIOD` (string duration, default: `30d`)
  - Duration between soft-delete and permanent purge. Supported formats: `30d`, `7d`, `24h`, or seconds.

- `PROGRESSDB_RETENTION_SCHEDULE` (string, default: `0 2 * * *`)
  - Cron expression (or a simple interval like `24h`) indicating when scheduled runs occur.

- `PROGRESSDB_RETENTION_BATCH_SIZE` (int, default: `1000`)
  - Max number of items to process per batch.

- `PROGRESSDB_RETENTION_DRY_RUN` (boolean, default: `false`)
  - When true, the worker logs which items would be deleted but does not perform deletes.

- `PROGRESSDB_RETENTION_MAX_RETRIES` (int, default: `3`)
  - Number of retries for transient failures on a batch or per-item delete.

- `PROGRESSDB_RETENTION_PAUSED` (boolean, default: `false`)
  - When true, scheduled runs skip purge processing.

- `PROGRESSDB_RETENTION_PAUSE_MS` (int, default: `100`)
  - Milliseconds to sleep between batches to reduce load (optional).

- `PROGRESSDB_RETENTION_MIN_PERIOD` (string, optional safety floor, e.g. `1h`)
  - Optional minimum allowed retention to guard against accidental tiny retention values.

Also add corresponding fields to the server config (YAML/JSON) under a `retention:` block, e.g.:

```yaml
retention:
  enabled: true
  period: "30d"
  schedule: "0 2 * * *"
  batchSize: 1000
  dryRun: false
  maxRetries: 3
  paused: false
  pauseMs: 100
  minPeriod: "1h"
```

Batch sizing and tuning guidance
-------------------------------

`PROGRESSDB_RETENTION_BATCH_SIZE` controls how many candidate items the worker processes
per loop. It is not the number of physical rows deleted — if a thread cascades many
messages/versions the effective rows deleted per item can be large. Tuning guidance:

- Default: `1000` — reasonable starting point for many deployments.
- Small systems: `100–500`.
- Large systems or where each item deletes many rows: `100–500`.
- Very large/fast cleanup where DB can handle it: `5k–50k` (use with caution).

Pick a conservative initial value, run a dry-run in staging, measure transaction time,
lock contention, WAL growth, and replication lag, then increase/decrease the batch size.

Why batching, not “delete everything one-by-one”?
-----------------------------------------------

Deleting everything in a single uncontrolled pass is fragile:
- It can take very long and may be interrupted.
- It makes it hard to control DB load and replication impact.
- Failure/recovery is complex without checkpoints.

Batches provide bounded work, easier retries, and more predictable operational impact.

Safety, idempotency and transactions
-----------------------------------

1. Prefer per-batch or per-item short transactions to keep rollback cost low.
2. Make deletes idempotent — running the purge again should be safe (no error if item missing).
3. Implement retries with exponential/backoff for transient errors and a clear failure policy
   for persistent errors (log and move on, alert operators).
4. For cross-store deletions (e.g., primary DB + search index + object storage), orchestrate
   per-item multi-step deletes that can be retried idempotently. Avoid a single huge transaction
   that spans multiple systems.
5. Use deterministic ordering (e.g., `ORDER BY deleted_at, id`) so progress is predictable.

Observability & operator controls
--------------------------------

Expose logs and metrics so operators can observe retention activity and adjust parameters.

Suggested metrics
- `retention.purged.count` — total items purged
- `retention.purged.threads.count`, `retention.purged.messages.count` — per-type counters
- `retention.errors.count` — number of errors encountered
- `retention.last_run_timestamp` — timestamp of last run
- `retention.last_duration_ms` — duration of last run

Suggested logs
- `retention: start` / `retention: finished` with counts
- `retention: purged_item` entries when running at debug level
- `retention: error` with details for failed batches

Operator controls
- Admin endpoint: `POST /admin/retention/run` (admin key required) to trigger a manual run.
- Support `dryRun` via admin endpoint (e.g. `POST /admin/retention/run?dryRun=true`).
- `paused` toggle (env or config) to temporarily disable scheduled runs.

Testing guidance
-----------------

- Unit tests: purge cutoff calculation, batch selection logic, and behavior when `dryRun` enabled.
- Mocked integration tests: mock store to assert correct delete calls for batches and cascading deletes.
- Integration tests with a test DB: create items, soft-delete them, set short retention, run worker, assert permanent removal.
- E2E: server + DB in CI to validate retention on a realistic stack.

Implementation checklist
------------------------

- [ ] Add `retention` config block and environment variable parsing (env > config > defaults).
- [ ] Add retention worker package with:
  - cutoff calculation
  - batch query and deterministic ordering
  - per-batch purge loop with retry/backoff
  - optional inter-batch pause
  - dry-run and pause handling
- [ ] Add store-level purge APIs or SQL statements that ensure cascade deletion of related data
      (implement using transactions where supported).
- [ ] Add scheduled runner (cron expression or interval) and/or a separate worker process entrypoint.
- [ ] Add admin HTTP endpoint for manual invocation and dry-run.
- [ ] Add metrics and logs and ensure they surface in your observability stack.
- [ ] Add unit/integration/e2e tests and CI job(s) to validate retention behavior.
- [ ] Document retention configuration and operational steps for operators.

Appendix: quick decision tree
- Is your dataset tiny (<< 10k deletions total)? Consider simple one-by-one.
- Do you have moderate or unknown scale, or production traffic? Use batching with small transactions.
- Do deletions cascade to many rows per item or involve multiple stores? Use smaller batches,
  per-item idempotent deletes, and consider a queue-based worker for resilience.

--

This document is intended to be the single source of truth for retention-related
engineering decisions. Keep it up to date as you iterate on implementation and
operational experience.

