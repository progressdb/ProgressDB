KMS Modes
---------

ProgressDB supports two KMS modes when `PROGRESSDB_USE_ENCRYPTION=true`:

- embedded (default): server runs with an in-process master key. Provide the
  master key via `security.kms.master_key_file` or `security.kms.master_key_hex`.
  Example:

  PROGRESSDB_USE_ENCRYPTION=true \
  PROGRESSDB_KMS_MODE=embedded \
  # or set in config: security.kms.master_key_file=... \
  ./server/cmd/progressdb

- external: the server talks to an already-running `kmsd` process over the
  configured socket (Unix domain socket or HTTP endpoint). The server will NOT
  provide the master key to the external KMS; the KMS daemon must hold and
  manage master key material.

  Example (start `kmsd` separately):

  # start kmsd (system/service or container)
  # then run server pointing at the socket
  PROGRESSDB_USE_ENCRYPTION=true \
  PROGRESSDB_KMS_MODE=external \
  PROGRESSDB_KMS_SOCKET=unix:///tmp/prog-kms.sock \
  ./server/cmd/progressdb

Notes
- `PROGRESSDB_USE_ENCRYPTION` must be `true` to enable KMS features.
- When `PROGRESSDB_KMS_MODE=embedded` the server will use AES-GCM with the
  configured master key and keep key material in process memory.
- When `PROGRESSDB_KMS_MODE=external` the server will not accept a master key
  from its configuration; instead it will communicate with the external KMS
  service over the socket specified by `PROGRESSDB_KMS_SOCKET`.

