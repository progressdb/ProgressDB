---
section: service
title: "Installation"
order: 1
visibility: public
---

# Installation

There are multiple ways to install and run ProgressDB: via Docker, a prebuilt
binary, or by building from source. Choose the option that best fits your
environment.

## Docker (quickest)

Pull the official Docker image and run it:

```sh
docker pull docker.io/progressdb/progressdb:latest
docker run -d \
  --name progressdb \
  -p 8080:8080 \
  -v $PWD/data:/data \
  docker.io/progressdb/progressdb --db /data/progressdb
```

This exposes the server on port `8080`. The admin viewer is available at
`http://localhost:8080/viewer/` and the Swagger UI at `http://localhost:8080/docs`.

## Prebuilt binary

Download a release binary from the Releases page and run it:

```sh
tar -xzf progressdb_<version>_linux_amd64.tar.gz
chmod +x progressdb
./progressdb --db ./data
```

On Windows, unzip and run `progressdb.exe`.

## Build from source (developer)

From the project root:

```sh
cd server
go run ./cmd/progressdb --db ./data --addr ":8080"
```

Use `--addr` and `--db` flags to override the listen address and database
location.

## SDKs & Clients

- Python backend SDK: `pip install progressdb`
- Node backend SDK: `npm install @progressdb/node`
- Frontend SDKs: `npm install @progressdb/js @progressdb/react`
