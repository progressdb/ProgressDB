# KMS

The KMS binary provides wrap/unwrap, DEK management, encrypt/decrypt, and KEK rotation over an HTTP-over-UDS API for ProgressDB.

## Prerequisites

- Go toolchain (recommended: `go 1.24+` to match module requirements).
- `openssl` available for generating dev KEKs (dev scripts use it).

## Build

From the repository root (builds the `progressdb-kms` binary into `./bin`):

```
cd kms
go build -o ../bin/progressdb-kms ./cmd/kms
```

Or using the module path directly:

```
go build -o bin/progressdb-kms ./cmd/kms
```

## Run (production)

Basic invocation:

```
bin/progressdb-kms --socket /var/run/progressdb-kms.sock --data-dir /var/lib/progressdb/kms
```


Startup secrets and allowed callers

- KMS accepts an optional `--config <path>` flag pointing to a YAML file that contains startup secrets and authorization configuration. When provided, KMS will read the master KEK file path and the allowed peer UIDs exclusively from that config file and will not read those values from environment variables. This is the recommended secure mode of operation.

Example `config.yaml` (self-contained):

```yaml
master_key_hex: "<64-hex-bytes>"
```

Notes:

- Wrapped DEKs and metadata are stored in a Pebble DB file at `<data-dir>/kms.db` (keys are namespaced, e.g. `meta:<keyid>`). Rotate operations still write per-key backup snapshots under `<data-dir>/kms-deks-backup/`.
- The KMS will identify the peer UID from UDS peer credentials for logging; it does not enforce an allowlist of caller UIDs. Restrict socket access via filesystem permissions for best security.

## Run (development)

There is a convenience dev script to build and run KMS locally (starts only the KMS):

```
./scripts/kms/dev.sh [--no-build] [--kms-bin <path>] [--socket <path>] [--data-dir <path>] [--mkfile <path>]
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

- If peer-credential checks fail, verify the server and KMS are running under expected UIDs and that socket filesystem permissions restrict access to the KMS socket appropriately.
- If `mlock` fails, check `ulimit -l` or systemd `LimitMEMLOCK` settings.

## Packaging

Dockerfile and a `systemd` unit are provided in the repository for convenience.
