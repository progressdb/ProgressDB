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

 - external: the server talks to an already-running `progressdb-kms` process over the
  configured endpoint (HTTP host:port). The server will NOT
  provide the master key to the external KMS; the KMS daemon must hold and
  manage master key material.

  Example (start `progressdb-kms` separately):

  # start progressdb-kms (system/service or container)
  # then run server pointing at the external HTTP endpoint
  PROGRESSDB_USE_ENCRYPTION=true \
  PROGRESSDB_KMS_MODE=external \
  PROGRESSDB_KMS_ENDPOINT=127.0.0.1:6820 \
  ./server/cmd/progressdb

Notes
- `PROGRESSDB_USE_ENCRYPTION` must be `true` to enable KMS features.
- When `PROGRESSDB_KMS_MODE=embedded` the server will use AES-GCM with the
  configured master key and keep key material in process memory.
 - When `PROGRESSDB_KMS_MODE=external` the server will not accept a master key
   from its configuration; instead it will communicate with the external KMS
   service over the endpoint specified by `PROGRESSDB_KMS_ENDPOINT`.
