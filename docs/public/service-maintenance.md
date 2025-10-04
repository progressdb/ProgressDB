---
section: service
title: "Maintenance"
order: 3
visibility: public
---

# Maintenance Runbook

This runbook provides concrete steps for backups, restores, KMS rotation,
monitoring, and common troubleshooting tasks.

## Backups

### What to back up

- Pebble DB directory (configured via `--db` / `storage.db_path`).
- KMS data directory and audit logs (when using embedded or external KMS).

### Filesystem snapshot procedure

1. Put the server into maintenance mode or stop it to ensure a consistent snapshot.
2. Create a filesystem snapshot (LVM/ZFS) or use `rsync` to copy the DB directory to backup storage.
3. Copy the KMS data directory and audit logs to the backup location.
4. Record metadata: backup time, binary version, config file used, and KMS key IDs.

### Verify backups

- Restore the snapshot to a staging host and start the server against the restored data.
- Run smoke tests: `/healthz`, create/list messages, check KMS decryption of encrypted fields.

## Restore procedure

1. Stop the target server.
2. Replace the `--db` directory with the backup snapshot.
3. Restore the KMS data directory and audit logs if applicable.
4. Start the server and validate `/healthz` and sample read/write flows.

## KMS rotation & rewrap (high level)

> Production recommendation: use `security.kms.mode: external` and run a
separate `progressdb-kms` service with restricted access.

### Rotation steps

1. Add the new KEK to the KMS and mark it active.
2. Use the KMS rewrap command to iterate wrapped DEKs and rewrap them with the new KEK.
3. KMS writes per-key backups into `kms-deks-backup/`; snapshot this directory.
4. Validate reads on a sample of threads/messages.
5. After a retention period and validation, retire old KEKs.

### Safety notes

- Always snapshot DB and KMS metadata before running a large rewrap.
- Keep KMS audit logs; they provide an auditable trail of rewrap operations.

## Monitoring & alerts

- Scrape `GET /metrics` with Prometheus.
- Suggested alerts:
  - `progressdb_health_status != 1`
  - High 5xx rate or error spikes
  - Disk usage on the DB path > 80%

## Troubleshooting

- Service does not start:
  - Check permissions on `--db` and KMS directories.
  - Inspect logs for config parsing errors.
- `/healthz` failing:
  - Ensure the service can reach the KMS (if enabled).
  - Check DB open errors in logs.
- Missing or unreadable encrypted fields:
  - Check KMS availability and KEK presence.

## Operational checklist

- [ ] Snapshot DB and KMS before upgrades.
- [ ] Verify backups by restoring to staging.
- [ ] Schedule maintenance windows for rewrap/migrations.

## Quick smoke test

1. Start server: `./progressdb --db ./data --addr :8080`.
2. Health: `curl -s http://localhost:8080/healthz` → expect `{ "status": "ok" }`.
3. Post message: `curl -X POST http://localhost:8080/v1/messages -H "Authorization: Bearer pk_example" -H "Content-Type: application/json" -d '{"thread":"smoke","author":"smoke","body":{"text":"smoke test"}}'`.

