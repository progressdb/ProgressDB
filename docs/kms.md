# KMS Installation & Spawn Guide

This document explains how to install the ProgressDB KMS binary and how the server spawns and connects to it. The KMS is implemented as a Go module and includes a small CLI at `cmd/progressdb-kms` so it can be installed as a standalone binary and launched as a separate process.

**Install:**
- **Go (recommended / development):** build the CLI locally and name the binary `progressdb-kms`:

  ```sh
  cd kms
  go build -o ../bin/progressdb-kms ./cmd/progressdb-kms
  ```

  Alternatively use Goreleaser or your packaging tool to produce `progressdb-kms` as a release artifact.
- **Release binary:** download the prebuilt `kms` artifact from the project releases and place it in `/usr/local/bin` (or another path in `PATH`).

**Service / process model**

- Embedded: the server includes an embedded KMS provider and, when `PROGRESSDB_KMS_MODE=embedded`, performs KMS operations in-process using the embedded library.
 - External: the server expects an external `progressdb-kms` process to be running and configured; when `PROGRESSDB_KMS_MODE=external` the server connects to the configured endpoint (HTTP host:port) to delegate KMS operations.

The server does not automatically spawn `progressdb-kms` by default. Operators who prefer the server to manage a child `progressdb-kms` process may implement that supervision externally (systemd, container runtime, or wrapper scripts). For external mode, ensure a `progressdb-kms` binary is available (install via `go install github.com/progressdb/kms/cmd/progressdb-kms@latest` or from releases) and start it before starting the server.

**Environment & configuration**
- `PROGRESSDB_USE_ENCRYPTION`: `true|1|yes` to enable encryption features in the server.
- `PROGRESSDB_KMS_BINARY`: deprecated — the server no longer reads this env var. Ensure `kms` is available on `PATH` or placed alongside the server executable.
- `PROGRESSDB_KMS_ENDPOINT`: Address used for server ↔ KMS communication. Must be a TCP host:port (e.g. `127.0.0.1:6820`) or a full URL (e.g. `http://kms.example:6820`). The server defaults to `127.0.0.1:6820` for external mode.
- `PROGRESSDB_KMS_DATA_DIR`: Directory where KMS stores metadata and logs (default `./kms-data`).
- Server config keys: `security.kms.master_key_hex` or `security.kms.master_key_file` — supply a master KEK for the KMS to use on startup. If not provided, KMS will generate an ephemeral master key (dev only).

**Systemd example**
Copy the `progressdb-kms` binary to `/usr/local/bin/progressdb-kms` (or install via `go install`) and create a systemd unit similar to the example below (a `progressdb-kms.service` example is included in the repo under the `kms/` directory):

```
[Unit]
Description=ProgressDB KMS
After=network.target

[Service]
Type=simple
User=progressdb
Group=progressdb
ExecStart=/usr/local/bin/progressdb-kms --endpoint 127.0.0.1:6820 --data-dir /var/lib/progressdb/kms --config /etc/progressdb/kms-config.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Ensure directories used by the KMS have restrictive permissions (e.g. `/var/lib/progressdb/kms` `700`). When using HTTP/TCP, ensure the endpoint is reachable only from trusted hosts and protected by network controls or transport auth (mTLS/tokens).

**Operational notes**
 - Authorization: protect the KMS HTTP endpoint using network-level controls (firewall / service mesh) or transport auth (mTLS, tokens). The server should not be relied upon to supply the master key in external mode.
 - Upgrade & install: tag a git release for the `github.com/progressdb/kms` module and use your release artifacts (or `go build -o`) to produce `progressdb-kms` for installation.
- Local development: the server includes a `replace` directive in its `go.mod` pointing to `../kms` for local testing. For production deployments, remove the `replace` and depend on published releases.

**Troubleshooting**
- If the server fails to start KMS: check `PATH` and ensure a `kms` binary is available, and check logs in the KMS data-dir for startup errors.
- If caller identity is required, run the KMS behind an authenticated transport (mTLS or tokens) or inside a trusted network segment.

For more details about the KMS internals and available HTTP endpoints, see `kms/README.md` and the `kms` package documentation in the repository.
