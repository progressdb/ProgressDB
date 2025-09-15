
# Encryption (KMS-based)

ProgressDB now requires an external KMS provider for encryption when `PROGRESSDB_USE_ENCRYPTION=true`.

Key points
- The server no longer accepts a raw symmetric key via `PROGRESSDB_ENCRYPTION_KEY` or `security.encryption_key` in config. That in-process mode has been removed.
- Use the KMS binary or an equivalent KMS provider; the server will spawn the KMS child when configured and `PROGRESSDB_USE_ENCRYPTION=true`.
- KMS responsibilities:
  - Hold the master KEK and perform audit-signed operations.
  - Provide endpoints for creating per-thread DEKs, rewrapping DEKs on rotation, and encrypt/decrypt operations over a unix-domain socket (UDS).

Configuration
- Environment variables used by the server to talk to KMS:
  - `PROGRESSDB_KMS_SOCKET` — UDS path (default `/tmp/progressdb-kms.sock`).
  - `PROGRESSDB_KMS_DATA_DIR` — directory where KMS stores metadata and audit logs.
  - The KMS master key should be provided via the server configuration; when the server spawns the KMS child it will embed the key into the child's `--config` YAML (self-contained). The environment variable `PROGRESSDB_KMS_MASTER_KEY_FILE` is removed.

API and behavior
- The server delegates encryption operations to KMS via a RemoteClient over UDS.
- API operations the KMS provides: create DEK for thread, get wrapped DEK, encrypt, decrypt, rotate KEK, rewrap a single key.

Operational notes
- KMS writes audit lines to `<data-dir>/kms-audit.log`; startup will fail if the audit file is not writable.
- Ensure socket directory permissions and ownership are correct so the server process can talk to KMS via peer-credentials.
- For rotation, use the KMS rotate endpoint which rewraps stored DEKs and updates KEK metadata.

References
- `server/docs/kms.md` for runbook and operational guidance about KMS setup.
- `kms/` directory contains the KMS implementation (binary and docs).
