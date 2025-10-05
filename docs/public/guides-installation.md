---
section: service
title: "Installation"
order: 1
visibility: public
---

# Installation

This page describes recommended ways to install and run ProgressDB for
development and production. Choose the option that best fits your
environment: Docker (quickest), prebuilt binaries, or building from source.

## Quick Docker example

Run a containerized instance (recommended for a quick local test):

```sh
docker pull docker.io/progressdb/progressdb:latest
docker run -d \
  --name progressdb \
  -p 8080:8080 \
  -v $PWD/data:/data \
  docker.io/progressdb/progressdb --db /data/progressdb
```

Endpoints available locally after startup:
- Health: `http://localhost:8080/healthz`

## Prebuilt binary

Download a release binary (Linux/macOS/Windows) from your release channel
and run it:

```sh
tar -xzf progressdb_<version>_linux_amd64.tar.gz
chmod +x progressdb
./progressdb --db ./data
```

On Windows, unzip and run `progressdb.exe`.

## Build from source (developer)

Clone the repo and run from the `server` directory:

```sh
cd server
go run ./cmd/progressdb --db ./data --addr ":8080"
```

Or build the binary:

```sh
cd server
go build -o progressdb ./cmd/progressdb
./progressdb --db ./data
```

## Command-line flags and environment variables

Common flags and their env var equivalents:

- `--db` / `--db-path` — storage path (env: `PROGRESSDB_DB_PATH`)
- `--addr` — bind address (env: `PROGRESSDB_ADDR` or `PROGRESSDB_ADDRESS`/`PROGRESSDB_PORT`)
- `--config` — YAML config file path (env: `PROGRESSDB_CONFIG`)
- `--tls-cert` / `--tls-key` — TLS cert and key paths (env: `PROGRESSDB_TLS_CERT`, `PROGRESSDB_TLS_KEY`)

Example: start with a config file

```sh
./progressdb --config ./configs/config.yaml
```

## Running behind Docker Compose

Use the provided example `docs/configs/docker-compose.yml` as a starting
point. Typical requirements:

- Mount a persistent volume for the DB path.
- Inject API keys and KMS config via environment variables or secrets.

## SDKs & clients

For information on available SDKs and client libraries (Python, Node, frontend, etc.), please refer to the **Clients** section of this documentation.

## Troubleshooting startup

- If the server fails to bind, check that the requested `--addr` is allowed on the host and not already in use.
- Check permissions on the DB path — the server process must be able to read/write the directory.
- Check the server logs (stdout or systemd journal) for detailed errors.