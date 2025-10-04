---
section: service
title: "Maintenance"
order: 3
visibility: public
---

# Maintenance Runbook

This runbook covers backups & restores, KMS rotation/rewrap, upgrades, and
common troubleshooting steps for ProgressDB.

Backups

- What to back up:
  - Pebble DB directory (the path set by `--db` / `storage.db_path`).
  - KMS data directory when using embedded KMS or the `progressdb-kms` data_dir when external.
  - KMS audit logs (`kms-audit.log`) and any exported key backups.

- Backup procedure (file-system snapshot):
  1. Stop the server or put it into maintenance mode (drain requests).
  2. Create a filesystem snapshot of the `--db` directory (e.g. `rsync` or LVM/ZFS snapshot).
  3. Copy the KMS data directory and audit logs to the backup location.
  4. Verify the backup by mounting/restoring into a staging host and starting the server against the restored data.

Restore (quick)

1. Stop the target server.
2. Replace the `--db` directory with the snapshot.
3. Restore the KMS data directory if needed.
4. Start the server and validate `/healthz` and core flows.

KMS rotation & rewrap

- Recommended mode: run an external `progressdb-kms` process and restrict access to it via network controls.
- Rotation steps (high level):
  1. Add the new KEK in the KMS and mark it as the active key.
  2. Run the KMS rewrap tooling (the KMS will iterate wrapped DEKs and rewrap them with the new KEK). Backups of wrapped-DEKs are written to `kms-deks-backup/`.
  3. Verify that decryption works by exercising reads on a subset of threads/messages.
  4. Remove the old KEK from rotation after verification and retention window.

- Operational notes:
  - Always snapshot KMS wrapped-DEK metadata before large rewraps.
  - Keep KMS audit logs; ensure the audit file is writable by the KMS process.

Upgrades

- Pre-upgrade checklist:
  - [ ] Backup DB and KMS data directories.
  - [ ] Verify backups by restoring to staging.
  - [ ] Review release notes for any breaking changes or migration steps.

- Rolling upgrade guidance:
  - Use health checks (`GET /healthz`) and readiness probes in your orchestrator.
  - Deploy new instances, wait for `healthz` to return `ok`, and then drain and stop old instances.
  - Keep the previous binary/image available for quick rollback.

- If an upgrade requires a data migration or rewrap, schedule a maintenance window and follow the migration/rewrap runbook.

Troubleshooting

- Service not starting:
  - Check logs for errors (`logs/` or stdout). Common issues: DB path not writable, invalid config, missing KMS endpoint.
  - Verify file permissions for `--db` and KMS directories.

- `/healthz` failing:
  - Inspect server logs for initialization errors.
  - If KMS is configured, ensure the KMS client can reach the endpoint and that credentials are valid.

- Missing messages or read errors:
  - Ensure the Pebble DB directory is intact and there are no partial writes (disk full, I/O errors).
  - Check KMS decryption errors in logs if encrypted payloads are failing to decrypt.

- KMS errors:
  - Check KMS audit logs and the KMS service status.
  - Ensure the KMS master key is present (for embedded) or the `progressdb-kms` service is running and accessible (for external).

Operational tips

- Monitoring: scrape `/metrics` with Prometheus and create alerts for:
  - `progressdb_health_status != 1`
  - Request error rate `5xx` spikes
  - DB disk usage and open file descriptors

- Logging: run the service under a process manager (systemd) and retain logs for at least one retention period.

- Security: protect API keys, the KMS service, and backup snapshots using your secrets manager and network ACLs.

Appendix: quick smoke test steps

1. Start server: `./progressdb --db ./data --addr :8080`.
2. Verify health: `curl -s http://localhost:8080/healthz` → `{ "status": "ok" }`.
3. Post a message (public key or signed user):

```sh
curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer pk_example" \
  -H "Content-Type: application/json" \
  -d '{"thread":"smoke","author":"smoke","body":{"text":"smoke test"}}'
```

4. Confirm metrics: `curl http://localhost:8080/metrics` and ensure exporter responds.

