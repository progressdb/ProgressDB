---
section: service
title: "Upgrades"
order: 4
visibility: public
---

# Upgrades

Short, practical guidance for replacing the executed binary or Docker image. This is not an exhaustive runbook — it assumes you follow the install guidance in `docs/public/guides-installation.md` and have backups in place.

## Pre-upgrade (always do these)

- Backup the Pebble DB directory (configured via `--db` / `storage.db_path`).
- Snapshot or copy KMS metadata / data (if using embedded KMS) or record external KMS configuration and keys.
- Record the currently running binary or container image tag so you can redeploy the previous version.

## Upgrade options

### Binary-based upgrade

1. Stop the running service (systemd/unit, supervisor, or kill the process).
2. Keep a copy of the current binary (move or rename `progressdb` to `progressdb.old` or save the current release artifact).
3. Replace the binary with the new `progressdb` executable in the same path and ensure executable permissions.
4. Start the service and verify health (see verification below).

### Container/image-based upgrade

1. Pull the new image and note the previous image tag.
2. Stop the old container and start a new container with the same volumes and config but the new image tag (or use your orchestrator's update mechanism).
3. Verify health (see verification below).

## Verification

- Check the health endpoint: `curl -s http://<host>:<port>/healthz` — expect `{ "status": "ok" }`.
- Run a simple smoke test (create a thread or post a short message) against a staging/canary instance before wide rollout.

## Rollback

- If the new version shows failures, redeploy the previous binary or image tag you recorded.
- If an incompatible data migration was performed and you cannot roll forward safely, restore the DB from the pre-upgrade backup and restart the previous version.

## Notes

- Keep upgrades simple and aligned with how you installed the service (binary vs container).
- Do not assume automatic migrations: if a release requires a migration that changes on-disk formats, treat it as a manual procedure and test it in staging first.
- For KMS: if using `embedded` mode, ensure you have a secure backup of the master key; if using `external` mode, ensure the external KMS endpoint and credentials are preserved.

Auto migrations will be added soon.
