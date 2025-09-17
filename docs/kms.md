# KMS Installation & Spawn Guide

This document explains how to install the ProgressDB KMS binary and how the server spawns and connects to it. The KMS is implemented as a Go module and includes a small CLI at `cmd/kms` so it can be installed as a standalone binary and launched as a separate process.

**Install:**
- **Go (recommended / development):** run `go install github.com/progressdb/kms/cmd/kms@latest`. Ensure your `GOBIN` or `$(go env GOPATH)/bin` is in your `PATH`.
- **Release binary:** download the prebuilt `kms` artifact from the project releases and place it in `/usr/local/bin` (or another path in `PATH`).

**Service / process model**
  - The server expects a KMS process to be available when encryption is enabled. When starting, the server will resolve which `kms` binary to use in the following order:
  - Prefer an installed `kms` binary found on `PATH` (i.e. `which kms`).
  - Otherwise, fall back to a sibling `kms` binary next to the ProgressDB executable (development fallback).
  - The server does not consult a `PROGRESSDB_KMS_BINARY` environment variable.
- Once the binary is determined, the server will create a small secure config file (master key, socket path, data-dir) and spawn the KMS as a child process. The server may also choose to prebind the UDS socket and pass the listener to the child to preserve peer credential behavior.

**Environment & configuration**
- `PROGRESSDB_USE_ENCRYPTION`: `true|1|yes` to enable encryption features in the server.
- `PROGRESSDB_KMS_BINARY`: deprecated — the server no longer reads this env var. Ensure `kms` is available on `PATH` or placed alongside the server executable.
- `PROGRESSDB_KMS_SOCKET`: Unix socket path used for server ↔ KMS communication (default `/tmp/progressdb-kms.sock`).
- `PROGRESSDB_KMS_DATA_DIR`: Directory where KMS stores metadata and logs (default `./kms-data`).
- Server config keys: `security.kms.master_key_hex` or `security.kms.master_key_file` — supply a master KEK for the KMS to use on startup. If not provided, KMS will generate an ephemeral master key (dev only).

**Systemd example**
Copy the `kms` binary to `/usr/local/bin/kms` (or install via `go install`) and create a systemd unit similar to the example below (a `kms.service` example is included in the repo under the `kms/` directory):

```
[Unit]
Description=ProgressDB KMS
After=network.target

[Service]
Type=simple
User=progressdb
Group=progressdb
ExecStart=/usr/local/bin/kms --socket /var/run/progressdb/kms.sock --data-dir /var/lib/progressdb/kms --config /etc/progressdb/kms-config.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Ensure directories used by the KMS have restrictive permissions (e.g. `/var/lib/progressdb/kms` `700`, and socket `0600`). The KMS binary will create files with `0600` where appropriate.

**Operational notes**
- Authorization: when KMS runs as a separate process bound to a Unix Domain Socket, the KMS inspects peer credentials (UID) for logging. If you choose to run KMS in-process (not recommended for production), you must rely on the server's auth middleware instead.
- Upgrade & install: tag a git release for the `github.com/progressdb/kms` module and use `go install github.com/progressdb/kms/cmd/kms@vX.Y.Z` to install a specific release.
- Local development: the server includes a `replace` directive in its `go.mod` pointing to `../kms` for local testing. For production deployments, remove the `replace` and depend on published releases.

**Troubleshooting**
- If the server fails to start KMS: check `PATH` and ensure a `kms` binary is available, and check logs in the KMS data-dir for startup errors.
- If peer UID is not available: ensure the server is passing an inherited UDS listener to the child process, or run KMS as a separate service and configure the server to use that socket.

For more details about the KMS internals and available HTTP endpoints, see `kms/README.md` and the `kms` package documentation in the repository.
