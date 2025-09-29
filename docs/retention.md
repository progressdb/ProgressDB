Retention Feature
=================

Overview
--------

The retention subsystem is an in-process, autorun background worker that
permanently purges soft-deleted threads and messages (and their related
versions/attachments) after a configurable retention period. The runner is
cron-driven and emits structured audit events via the application logger;
the logger can be configured to sink audit output to a file-based audit
folder which external backup tooling may compress and archive.

Key semantics
-------------

- Soft-delete means the item has `Deleted = true` and `DeletedTS` set.
- An item is eligible for permanent purge when `deleted_at + retention_period <= now()`.
- Purges run in batches and use a coordinator lease to ensure only one
  instance performs a run at a time.
- Audit events are emitted to the global logger. Configure `PROGRESSDB_LOG_SINK`
  to a file sink pointing at `PROGRESSDB_RETENTION_AUDIT_PATH` (defaults to
  `<DBPath>/purge_audit/YYYY-MM/`) so that audit output is written to disk
  where backup tooling can pick it up.
- By default retention is disabled; enable deliberately and test with
  `dry_run=true` first.

Safety & defaults
-----------------

- Default: `PROGRESSDB_RETENTION_ENABLED=false` (OFF by default).
- Default retention period: `PROGRESSDB_RETENTION_PERIOD=30d`.
- Default cron schedule: `PROGRESSDB_RETENTION_CRON="0 2 * * *"` (daily @02:00 UTC).
- Minimum allowed retention (`min_period`) default: `1h`. Shorter values are
  rejected on startup unless explicitly allowed via
  - `dry_run=true` writes audit entries but does not delete data.

Audit files
-----------

- Audit output is emitted as structured JSON events with messages like
  `retention_audit_header`, `retention_audit_item`, and `retention_audit_footer`.
  Each run emits audit events to the audit logger (audit.log).

Coordinator lease
-----------------

The system uses a file-based lease located at the audit path by default
(`retention.lock`). The lease contains `owner` and `expires` fields; the
runner acquires the lease before starting work and heartbeats to renew it
periodically. If the lease cannot be acquired the runner skips the run.

Purge behavior
--------------

- The runner scans thread metadata and selects candidates where
  `Deleted == true` and `DeletedTS <= now - retention_period`.
- For thread purges the worker deletes keys under the `thread:<id>:`
  prefix (messages, versions, thread meta). For messages there is a
  `version:msg:<msgID>:` index that is also removed.
- Deletes are idempotent — re-running the same purge will not cause errors
  for absent rows.
- For threads with very large message sets the initial implementation deletes
  keys individually; chunked deletes may be added for production scale.

Operational guidance
--------------------

1. Test in staging with `PROGRESSDB_RETENTION_ENABLED=true` and
   `PROGRESSDB_RETENTION_DRY_RUN=true` to inspect audit files before
   enabling destructive deletions.
2. Backup DB before enabling purges in production.
3. Start with conservative defaults (30d) and only lower the retention in
   isolated test environments.

Migration notes
---------------

Ensure your thread and message metadata include soft-delete fields:

- Threads: `Deleted bool`, `DeletedTS int64` (ns)
- Messages: `Deleted bool` (if tombstones are used)

If these fields are missing, add them via migration before enabling
retention.

Files changed / where code lives
-------------------------------

- Retention code: `server/internal/retention/` (scheduler, lease, runner, audit writer).
- Store purge hooks: `server/pkg/store/pebble.go` (`PurgeThreadPermanently`, `PurgeMessagePermanently`).
- Config parsing & validation: `server/pkg/config/*`, startup validation in `server/internal/app/validate.go`.

