---
section: service
title: "Upgrades"
order: 4
visibility: public
---

# Upgrades & Releases

This guide outlines safe upgrade procedures, rolling upgrade patterns,
backup and rollback steps, and a release checklist.

## Pre-upgrade checklist

- [ ] Backup Pebble DB (`--db`) and KMS metadata.
- [ ] Restore backups to staging and run smoke tests.
- [ ] Review release notes for breaking changes and data migrations.
- [ ] Ensure previous binaries/images are available for rollback.

## Rolling upgrades (recommended)

1. Deploy new version to a small canary subset of instances.
2. Wait for `/healthz` to return `ok` on each new instance.
3. Monitor metrics and logs for errors or latency regressions.
4. Gradually roll out to remaining instances.

Tips:

- Use readiness probes to avoid routing traffic to instances that are still warming up.
- Drain connections and stop old instances only after new instances are healthy.

## Upgrades that include data migrations

- If the upgrade requires data migrations (schema changes, encryption rewrap), schedule a maintenance window.
- Run migrations on a staging copy first and validate data integrity.
- Prefer online migrations that are backwards-compatible when possible; otherwise, use downtime windows.

## Quick rollback

If you detect problems post-upgrade:

1. Stop the new version and redeploy the previous known-good binary/image.
2. If data migrations made incompatible changes, restore DB from the pre-upgrade backup and restart the previous version.
3. Notify stakeholders and open an incident ticket with the timeline and logs.

## CI/CD recommendations

- Use `goreleaser` (configured in this repo) to build artifacts and Docker images.
- Publish artifacts with clear semantic tags and include release notes describing migrations and breaking changes.

## Post-upgrade verification

- Confirm `/healthz` is `ok` across all nodes.
- Verify metrics (`/metrics`) for error spikes or latency regressions.
- Run functional smoke tests: create thread, post message, list messages, sign user.

## Release checklist (copyable)

- [ ] Backup DB and KMS
- [ ] Deploy to staging and run automated tests
- [ ] Deploy to canary subset and monitor metrics/health
- [ ] Rollout to remainder of fleet
- [ ] Monitor for errors and latency regressions for at least one retention period

