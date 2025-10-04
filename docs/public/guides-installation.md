---
section: service
title: "Installation"
order: 1
visibility: public
---

# Installation

There are multiple ways to install and run ProgressDB: via Docker, a
prebuilt binary, or by building from source. Choose the option that best fits
your environment.

Docker (quickest)

Pull the official image and run a container:

```sh
docker pull docker.io/progressdb/progressdb:latest
docker run -d \
  --name progressdb \
  -p 8080:8080 \
  -v $PWD/data:/data \
  docker.io/progressdb/progressdb --db /data/progressdb
```

This exposes the server on port `8080`. Useful endpoints:

- Admin viewer UI: `http://localhost:8080/viewer/`
- API docs (Swagger/OpenAPI): `http://localhost:8080/docs/` and `GET /openapi.yaml`
- Health: `http://localhost:8080/healthz`

Prebuilt binary

Download a release binary from the Releases page and run it:

```sh
tar -xzf progressdb_<version>_linux_amd64.tar.gz
chmod +x progressdb
./progressdb --db ./data
```

On Windows, unzip and run `progressdb.exe`.

Build from source (developer)

From the project root:

```sh
cd server
go run ./cmd/progressdb --db ./data --addr ":8080"
```

You can also build the binary with the provided scripts:

```sh
./scripts/build.sh
```

Flags and common environment variables

- `--db` / env `PROGRESSDB_DB_PATH` — Pebble DB path (persistent directory).
- `--addr` / env `PROGRESSDB_ADDR` — listen address (e.g. `0.0.0.0:8080`).
- `--config` / env `PROGRESSDB_CONFIG` — path to YAML config file (`docs/configs/config.yaml` contains an example).

SDKs & clients

- Python backend SDK: `pip install progressdb` (repository contains the SDK layout under `clients/`).
- Node backend SDK: `npm install @progressdb/node`.
- Frontend SDKs: `npm install @progressdb/js @progressdb/react`.

Operational notes

- For production, set a persistent DB path via `--db` and secure the server with API keys and TLS.
- If you enable encryption, run an external KMS (`progressdb-kms`) and configure `security.kms.mode=external`.
- Use the Prometheus metrics endpoint `GET /metrics` for monitoring.

