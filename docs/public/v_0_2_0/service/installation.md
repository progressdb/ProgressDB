---
section: service
title: "Installation"
order: 1
visibility: public
---

# Installation

## Docker (recommended)
```sh
docker run -d --name progressdb -p 8080:8080 -v $PWD/data:/data \
  docker.io/progressdb/progressdb --db /data/progressdb
```

## Prebuilt binary
```sh
tar -xzf progressdb_<version>_linux_amd64.tar.gz
./progressdb --db ./data
```

## Build from source
```sh
cd server
go run ./cmd/progressdb --db ./data --addr ":8080"
```

## Common flags
- `--db` - storage path
- `--addr` - bind address  
- `--config` - YAML config file

Health check: `http://localhost:8080/healthz`

Configuration: See [configuration](/configuration) for all available options.
```
