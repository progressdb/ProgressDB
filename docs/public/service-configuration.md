---
section: service
title: "Configuration"
order: 2
visibility: public
---

# Configuration

ProgressDB can be configured via a YAML config file (`config.yaml`) or
environment variables. For local development there is an `env.example` file in
the `server` directory.

Common configuration keys

- `--db` / `db.path` — data directory for Pebble storage.
- `--addr` / `server.addr` — listen address (default `:8080`).
- `security.kms.*` — KMS-related options (master key path, external KMS endpoint).
- `auth.*` — API key and signing options for admin/backends.

Example (minimal `config.yaml`):

```yaml
server:
  addr: ":8080"
storage:
  db: ./data
security:
  kms:
    mode: embedded
```

Advanced

- Use environment variables in CI or orchestrator (e.g., `PROGRESSDB_CONFIG`)
  to point to a config file.
- Protect config files and secrets (API keys, KEK) with strict filesystem
  permissions and use a secrets manager in production.

