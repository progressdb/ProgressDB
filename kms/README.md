# KMS

This repository directory contains the KMS integrator used by ProgressDB. It provides wrap/unwrap, DEK management, encrypt/decrypt, and KEK rotation over an HTTP-over-UDS API.

## Build

Requires Go 1.20+. From the repository root:

```
cd cmd/kms
go build -o kms
```

## Run

Environment variables:
- `PROGRESSDB_KMS_MASTER_KEY` - optional 64-hex KEK; if omitted an ephemeral KEK is generated (dev only).
- `PROGRESSDB_KMS_ALLOWED_UIDS` - comma-separated numeric UIDs allowed to connect via UDS peer credentials.

Run:

```
./kms --socket /var/run/progressdb-kms.sock --data-dir /var/lib/progressdb/kms
```

## Packaging

Dockerfile and a `systemd` unit are provided in the repository for convenience.

