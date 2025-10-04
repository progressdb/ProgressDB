---
section: service
title: "Maintenance"
order: 3
visibility: public
---

# Maintenance

Operational guidance for running ProgressDB in production.

Backups

- Periodically snapshot the `--db` directory and test restores regularly.
- For encrypted data, ensure KMS backup and key rotation policies are in place.

Monitoring

- Scrape `http://<host>:8080/metrics` with Prometheus.
- Monitor service uptime, request error rates, and disk usage.

Upgrades

- Stop the service gracefully, backup data, and run the new binary with the
  same `--db` path. Validate on a staging environment before production.

Troubleshooting

- Check server logs in `logs/` and the admin viewer at `/viewer/`.
- For KMS issues, consult the KMS runbook and rotation logs.

