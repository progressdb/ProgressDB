# ProgressDB CLI (prgcli)

Command-line tool for ProgressDB database migration and benchmarking.

# Install

### Direct Download
```bash
# macOS/Linux
curl -L https://github.com/ProgressDB/progressdb/releases/latest/download/prgcli_{{version}}_{{os}}_{{arch}}.tar.gz | tar xz
sudo mv prgcli /usr/local/bin/

# Windows
# Download prgcli_{{version}}_windows_amd64.zip and extract
```

### Package Managers
```bash
# Homebrew (macOS/Linux)
brew install progressdb/prgcli

# Scoop (Windows)
scoop install progressdb/prgcli
```

# Commands

## migrate

```bash
# Using old service config (recommended)
prgcli migrate --old-config /path/to/old/config.yaml --to /new/db

# Manual configuration
prgcli migrate --old-db-path /old/db --old-encryption-key <key> --to /new/db

# Interactive mode
prgcli migrate --to /new/db

# With config file
prgcli migrate --config config.yaml
```

## benchmark

```bash
# Quick benchmark
prgcli benchmark --auto --duration 30s

# Custom settings
prgcli benchmark --host http://localhost:8080 --rps 500 --duration 2m --backend-key sk_example --frontend-key pk_example --user user1
```