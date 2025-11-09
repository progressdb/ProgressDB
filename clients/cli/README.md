# ProgressDB CLI

A command-line tool for managing ProgressDB databases, including migration between versions and performance benchmarking.

## Features

- **Database Migration**: Migrate ProgressDB databases from version 0.1.2 to 0.5.0
- **Performance Benchmarking**: Test ProgressDB service throughput and latency
- **Interactive Configuration**: Prompt for missing values instead of requiring all flags
- **Config File Support**: Use YAML configuration files for repeated migrations
- **Pebble Database Copy**: Copy pebble database files safely
- **Encryption Support**: Handle old 0.1.2 AES-256-GCM encryption

## Installation

```bash
# Build from source
cd clients/cli
go build -o progressdb .

# Or install directly
go install -o progressdb .
```

## Usage

### Database Migration

```bash
# Interactive mode - will prompt for missing values
./progressdb migrate --from /old/db --to /new/db

# With config file
./progressdb migrate --from /old/db --to /new/db --config config.yaml

# Verbose output
./progressdb migrate --from /old/db --to /new/db --verbose
```

### Benchmark

```bash
# Quick benchmark
./progressdb benchmark --auto --duration 30s

# Custom settings
./progressdb benchmark --host http://localhost:8080 --rps 500 --duration 2m
```

### Performance Benchmarking

```bash
# Auto mode with defaults (only prompts for pattern)
./progressdb benchmark --auto

# Full configuration
./progressdb benchmark --host http://localhost:8080 --backend-key sk_your_key --frontend-key pk_your_key --rps 1000 --duration 2m

# Quick test with specific pattern
./progressdb benchmark --auto --pattern create_threads --duration 30s

# Verbose benchmarking
./progressdb benchmark --auto --verbose --duration 1m
```

#### Benchmark Patterns

1. **create_threads**: Creates new threads continuously to test thread creation throughput
2. **thread_with_messages**: Creates one thread then sends messages to it to test message throughput

#### Benchmark Output

The benchmark generates:
- Live statistics during execution (RPS, response times, remaining time)
- JSON output file with detailed metrics in `./logs/bench-{timestamp}.json`
- Percentile calculations (P90, P95, P99) for response times

### Configuration File

Create a YAML configuration file:

```yaml
# config.yaml
old_encryption_key: "your-32-byte-hex-encryption-key-here"
from_database: "/path/to/old/progressdb"
to_database: "/path/to/new/progressdb"
```

Then use it:

```bash
./progressdb migrate --config config.yaml
```

### Interactive Prompts

If you don't provide a config file, the CLI will prompt for missing values:

```bash
./progressdb migrate --from /old/db --to /new/db
```

This will prompt for:
- Old encryption key (masked input)
- Confirmation before migration

## Migration Process

The CLI performs these steps:

1. **Copy Pebble Files**: Copies pebble database files from source to target
2. **Decrypt Data**: Decrypts old 0.1.2 AES-256-GCM encrypted data
3. **Migrate Format**: Converts data from 0.1.2 format to 0.5.0 format
4. **Create Indexes**: Generates performance indexes for the new format
5. **System Info**: Creates migration metadata

### Output Structure

The migration creates this directory structure:

```
new/db/
├── storedb/
│   ├── threads/     # Thread metadata files
│   └── messages/    # Message content files
├── indexdb/
│   └── indexes/     # Performance indexes
└── system.json       # System metadata
```

## Requirements

- Go 1.24 or later
- For migration: Source database must be a valid ProgressDB 0.1.2 database
- For migration: Old encryption key (32-byte hex string) from the original 0.1.2 setup
- For benchmarking: Running ProgressDB service at specified host

## Security Notes

- The old encryption key is handled securely with masked input
- Keys are validated to be exactly 32 bytes (64 hex characters)
- No sensitive data is logged in verbose mode

## Examples

### Migrate with Config File

```bash
# Create config
cat > migration.yaml << EOF
old_encryption_key: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
from_database: "/var/lib/progressdb/old"
to_database: "/var/lib/progressdb/new"
EOF

# Run migration
./progressdb migrate --config migration.yaml --verbose
```

### Interactive Migration

```bash
# Will prompt for encryption key
./progressdb migrate --from /old/db --to /new/db

# Example prompts:
# Enter old encryption key (hex, 32 bytes): ****
# Confirm encryption key: ****
# Do you want to proceed with the migration? [y/N]: y
```

## Troubleshooting

### Common Issues

1. **"Invalid encryption key"**: Ensure your key is exactly 64 hex characters (32 bytes)
2. **"Source database does not exist"**: Check the source path and permissions
3. **"Failed to decrypt data"**: Verify you're using the correct encryption key from 0.1.2

### Debug Mode

Use `--verbose` flag to see detailed progress and error information:

```bash
./progressdb migrate --from /old/db --to /new/db --verbose
```

## Development

### Building

```bash
cd clients/cli
go build -o progressdb .
```

### Testing

```bash
# Test with sample data
./progressdb migrate --from ./test-data/old --to ./test-data/new --verbose
```

## License

Same license as ProgressDB project.