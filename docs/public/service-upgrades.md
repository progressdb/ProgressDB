---
section: service
title: "Upgrades"
order: 4
visibility: public
---

# Upgrades & Releases

This page describes recommended procedures for upgrading ProgressDB in
production and best practices for releases.

## Rolling upgrades

- Use health checks (`GET /healthz`) and readiness probes to orchestrate
  rollouts. Ensure each new instance reports healthy before routing traffic.
- Drain connections on an instance before stopping it (graceful shutdown).
- Keep previous released binaries/images available for rollback.

## Backup before upgrade

- Always snapshot the `--db` data directory (Pebble DB) before upgrading.
- Verify backups by restoring into a staging environment and running smoke
  tests against the restored instance.

## Compatibility & migrations

- The server attempts to maintain API compatibility within a release line.
- If an upgrade includes data migrations (schema changes, encryption rewrap),
  run migrations in a maintenance window and validate sample data after the
  migration completes.

## CI/CD & release artifacts

- Use `goreleaser` (repo provides `.goreleaser.yml`) to build artifacts and
  Docker images. Tag releases and keep release notes that document any
  breaking changes.
- Build artifacts should include a versioned binary and Docker image for each
  release.

## Quick rollback

- If you detect issues post-upgrade, rollback to the previous known-good
  binary/image and restore from the backup snapshot if necessary.

## Post-upgrade verification

- Confirm `/healthz` is `ok` on all nodes.
- Verify metrics (`/metrics`) show expected request rates and no error
  spikes.
- Exercise a few end-to-end scenarios: create thread, post message, list
  messages, sign user.

## Release checklist

- [ ] Backup DB
- [ ] Deploy new version to staging and run automated tests
- [ ] Deploy to canary subset and monitor metrics/health
- [ ] Rollout to remainder of fleet
- [ ] Monitor for errors and latency regressions for at least one
  retention/window period

