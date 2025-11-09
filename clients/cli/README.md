# ProgressDB CLI

Command-line tool for ProgressDB database migration and benchmarking.

## Features

- **Database Migration**: Migrate from ProgressDB 0.1.2 to 0.5.0
- **Performance Benchmarking**: Test service throughput and latency
- **Interactive Configuration**: Prompt for missing values
- **Config File Support**: Use YAML configuration files

## Installation

```bash
cd clients/cli
go build -o progressdb .
```

## Usage

### Migration

```bash
# Using old service config (recommended)
./progressdb migrate --old-config /path/to/old/config.yaml --to /new/db

# Manual configuration
./progressdb migrate --old-db-path /old/db --old-encryption-key <key> --to /new/db

# Interactive mode
./progressdb migrate --to /new/db
```

### Benchmarking

```bash
# Quick benchmark
./progressdb benchmark --auto --duration 30s

# Custom settings
./progressdb benchmark --host http://localhost:8080 --rps 500 --duration 2m
```

## Configuration

Create a migration config file:

```yaml
old_encryption_key: "your-32-byte-hex-key"
from_database: "/path/to/old/db"
to_database: "/path/to/new/db"
output_format: "json"
```

Use with: `./progressdb migrate --config config.yaml`

## Migration Methods

1. **Old Config File**: `--old-config` - Auto-loads from 0.1.2 service config
2. **Manual Flags**: `--old-db-path`, `--old-encryption-key` - Specify directly
3. **Config File**: `--config` - Use dedicated migration config
4. **Interactive**: Omit parameters and be prompted

## Requirements

- Go 1.24+
- Source: ProgressDB 0.1.2 database with encryption key
- Target: Empty directory for new database

## Examples

See `MIGRATION_EXAMPLES.md` for detailed usage examples.

## License

Same as ProgressDB project.