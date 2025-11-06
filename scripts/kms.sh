#!/bin/bash

# KMS Runner Script
# Usage: ./scripts/kms [args...]

set -e

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
KMS_DIR="$PROJECT_ROOT/kms"

# Check if KMS directory exists
if [ ! -d "$KMS_DIR" ]; then
    echo "Error: KMS directory not found at $KMS_DIR"
    exit 1
fi

# Default config file
CONFIG_FILE="$SCRIPT_DIR/cfkms.yaml"

# Check if config file exists
if [ ! -f "$CONFIG_FILE" ]; then
    echo "Error: KMS config file not found at $CONFIG_FILE"
    echo "Please create the config file or specify a different one with --config"
    exit 1
fi

# Change to KMS directory and run
cd "$KMS_DIR"

echo "Starting KMS server..."
echo "Config: $CONFIG_FILE"
echo "Working directory: $KMS_DIR"
echo ""

# Run KMS with the config file
exec go run ./cmd/prgkms/main.go -config "$CONFIG_FILE" "$@"