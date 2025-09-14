# miniKMS

miniKMS is a small, isolated key management service extracted from ProgressDB. It provides wrap/unwrap, DEK management, encrypt/decrypt, and KEK rotation over an HTTP-over-UDS API.

## Build

Requires Go 1.20+. From the repository root:

```
cd cmd/minikms
go build -o minikms
```

## Run

Environment variables:
- `PROGRESSDB_MINIKMS_MASTER_KEY` - optional 64-hex KEK; if omitted an ephemeral KEK is generated (dev only).
- `PROGRESSDB_MINIKMS_ALLOWED_UIDS` - comma-separated numeric UIDs allowed to connect via UDS peer credentials.

Run:

```
./minikms --socket /var/run/progressdb-minikms.sock --data-dir /var/lib/progressdb/minikms
```

## Packaging

Dockerfile and a `systemd` unit are provided in the repository for convenience.

