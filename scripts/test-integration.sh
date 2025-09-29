#!/usr/bin/env bash
set -euo pipefail

# Run server integration tests (tagged with 'integration')
echo "Running server integration tests..."
go test -tags=integration ./server/tests -v

