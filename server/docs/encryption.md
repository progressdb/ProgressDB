
# Encryption (KMS-based)

ProgressDB supports encryption when `PROGRESSDB_USE_ENCRYPTION=true` in two modes:

 - Embedded: the server binary includes the embedded provider and performs KMS operations in-process via the HashiCorp AEAD provider when `PROGRESSDB_KMS_MODE=embedded`.
 - External: the server communicates with a separate `progressdb-kms` process over HTTP/TCP and delegates KMS work to it when `PROGRESSDB_KMS_MODE=external`.

Key points
 - The server binary contains both embedded and external implementations; the runtime env `PROGRESSDB_KMS_MODE` selects which one is used.
 - Do not provide a master key to the server in external mode; the external `progressdb-kms` must hold the KEK.
- KMS responsibilities:
  - Hold the master KEK and perform audit-signed operations.
 - Provide endpoints for creating per-thread DEKs, rewrapping DEKs on rotation, and encrypt/decrypt operations over HTTP/TCP.

Configuration
- Environment variables used by the server to talk to KMS:
  - `PROGRESSDB_KMS_ENDPOINT` — address used to reach external KMS (host:port or full URL). Default is `127.0.0.1:6820`.
  - `PROGRESSDB_KMS_DATA_DIR` — directory where KMS stores metadata and audit logs.
 - The KMS master key should be provided via the server configuration for embedded mode. For external mode, the `progressdb-kms` daemon holds the master key; do not supply a master key to the server in that case.

API and behavior
 - The server delegates encryption operations to KMS via a RemoteClient over HTTP/TCP.
- API operations the KMS provides: create DEK for thread, get wrapped DEK, encrypt, decrypt, rotate KEK, rewrap a single key.

Operational notes
- KMS writes audit lines to `<data-dir>/kms-audit.log`; startup will fail if the audit file is not writable.
- Ensure the KMS endpoint is protected and reachable by the server; use mTLS, a private network, or firewall rules for access control.
- For rotation, use the KMS rotate endpoint which rewraps stored DEKs and updates KEK metadata.

References
- `server/docs/kms.md` for runbook and operational guidance about KMS setup.
- `kms/` directory contains the KMS implementation (binary and docs).
