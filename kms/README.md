# KMS

The KMS binary provides wrap/unwrap, DEK management, encrypt/decrypt, and KEK rotation over HTTP/TCP for ProgressDB.

## Prerequisites

- Go toolchain (recommended: `go 1.24+` to match module requirements).
- `openssl` available for generating dev KEKs (dev scripts use it).

## Build

From the repository root (builds the `progressdb-kms` binary into `./bin`):

```
cd kms
go build -o ../bin/progressdb-kms ./cmd/progressdb-kms
```

Or using the module path directly:

```
go build -o bin/progressdb-kms ./cmd/progressdb-kms
```

## Run (production)

Basic invocation (TCP):

```
bin/progressdb-kms --endpoint 127.0.0.1:6820 --data-dir /var/lib/progressdb/kms
```


Startup secrets and allowed callers

- KMS accepts an optional `--config <path>` flag pointing to a YAML file that contains startup secrets and authorization configuration. When provided, KMS will read the master KEK file path and the allowed peer UIDs exclusively from that config file and will not read those values from environment variables. This is the recommended secure mode of operation.

Example `config.yaml` (self-contained):

```yaml
master_key_hex: "<64-hex-bytes>"
```

Notes:

- Wrapped DEKs and metadata are stored in a Pebble DB file at `<data-dir>/kms.db` (keys are namespaced, e.g. `meta:<keyid>`). Rotate operations still write per-key backup snapshots under `<data-dir>/kms-deks-backup/`.
 - Protect the KMS HTTP endpoint using network controls or transport authentication (mTLS/tokens); the KMS records caller information in audit logs and does not itself enforce an allowlist.

## Run (development)

There is a convenience dev script to build and run KMS locally (starts only the KMS):

```
./scripts/kms/dev.sh [--no-build] [--kms-bin <path>] [--endpoint <addr>] [--data-dir <path>] [--mkfile <path>]
```

Examples:

- Build and run KMS (default `./bin/kms`):
  - `./scripts/kms/dev.sh`
- Run without building (use prebuilt `./bin/kms`):
  - `./scripts/kms/dev.sh --no-build`

## Memory hardening

- On supported Unix platforms the KMS locks KEK bytes in memory with `mlock` to reduce swap exposure. Ensure the KMS user has sufficient `RLIMIT_MEMLOCK` (e.g. `LimitMEMLOCK=infinity` in systemd or `ulimit -l` adjusted) for production.

## Logs & data

- Audit log: `<data-dir>/kms-audit.log` (append-only JSON lines with HMAC signature).
- Metadata DB: `<data-dir>/kms.db` (Pebble DB). Backups are written to `<data-dir>/kms-deks-backup/` when rotation/rewrap occurs.

## Troubleshooting

- If connectivity or authentication checks fail, verify the KMS HTTP endpoint is reachable and that transport authentication (mTLS, tokens) is configured correctly.
- If `mlock` fails, check `ulimit -l` or systemd `LimitMEMLOCK` settings.

## Packaging

Dockerfile and a `systemd` unit are provided in the repository for convenience.
