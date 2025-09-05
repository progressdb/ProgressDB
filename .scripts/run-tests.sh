#!/usr/bin/env bash
set -euo pipefail

# Run server package tests
echo "Running server tests..."
cd "$(dirname "$0")/.."
cd server
go test ./... -v

