---
section: service
title: "Maintenance"
order: 3
visibility: public
---

# Maintenance

Concrete backup, restore, and short verification steps for operators. This is intentionally minimal — it assumes you follow the install guidance in `docs/public/guides-installation.md` and that you manage the service as a binary or container per your environment.

Pre-maintenance checklist

- Ensure you have a recent backup of the Pebble DB directory (`--db` / `storage.db_path`).
- Snapshot or copy KMS metadata (if using embedded KMS) or verify external KMS configuration and credentials are recorded.
- Note the running binary path or container image tag for rollback purposes.

Backup procedure

- Preferred: create a filesystem snapshot (LVM, ZFS, etc.) of the DB path while the service is stopped or paused to ensure consistency.
- Alternative: stop the service and use `rsync -a` to copy the DB directory to backup storage.
- Also copy KMS files (embedded mode) or record external KMS settings (mode, endpoint, key IDs).

Verify backups

- Restore the backup to a staging host and start the service against the restored data.
- Confirm `/healthz` returns `{ "status": "ok" }` and run a single read/write smoke test.

Restore procedure

1. Stop the target service.
2. Replace the DB directory with the restored snapshot or backup copy.
3. Restore KMS files if applicable or ensure external KMS endpoint and credentials are available.
4. Start the service and verify `/healthz` and a basic smoke test.

KMS notes

- If using `embedded` KMS: securely back up the master key and KMS data directory before maintenance.
- If using `external` KMS: ensure the external KMS endpoint, credentials, and key IDs remain unchanged and reachable.
- For any rewrap or rotation, snapshot DB and KMS metadata first and validate reads on a staging restore.

Monitoring & quick checks

- Hit the health endpoint: `curl -s http://<host>:<port>/healthz` (expect `{ "status": "ok" }`).
- Check logs for DB open errors or KMS errors after restart.

Quick smoke test (example)

1. Start the server (binary example):

```sh
./progressdb --db ./data --addr :8080
```

2. Health check:

```sh
curl -s http://localhost:8080/healthz
# expect: { "status": "ok" }
```