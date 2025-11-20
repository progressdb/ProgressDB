---
section: service
title: "Installation"
order: 1
visibility: public
---

# Installation

ProgressDB is lightweight and efficient, with a build size of just ~50MB.  
- Hardware requirements depend on your workload—see our [Benchmarks](/docs/benchmarks) to estimate what you’ll need.
- For basic use (e.g., handling 100 requests per second), a setup with 512MB RAM, 1GB disk space, and a single vCPU is often sufficient.

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

## Links

- [Docker Images & Packages on GitHub](https://github.com/orgs/progressdb/packages?repo_name=ProgressDB)
- [Source Code on GitHub](https://github.com/progressdb/ProgressDB)
- [Releases & Binaries](https://github.com/progressdb/ProgressDB/releases)

## Common flags
- `--db` - storage path
- `--addr` - bind address  
- `--config` - YAML config file

Health check: `http://localhost:8080/healthz`

Configuration: See [configuration](/docs/configuration) for all available options.