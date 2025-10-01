# SDK Test Scripts

This directory contains helper scripts for running SDK tests across different environments.

Available test scripts:

- `test-node.sh` — Run Node.js SDK tests (backend).
  - Usage: `./scripts/sdk/test-node.sh [--unit|--integration|--all] [--watch]`

- `test-frontend.sh` — Run frontend SDK tests (TypeScript + React).
  - Usage: `./scripts/sdk/test-frontend.sh [--unit|--integration|--all] [--watch]`

- `test-all-sdks.sh` — Run all SDK tests sequentially.
  - Usage: `./scripts/sdk/test-all-sdks.sh [--watch]`

- `test-python.sh` — Run Python backend SDK tests (pytest + responses).
  - Installs `pytest` and `responses` into the active Python environment and runs tests under `clients/sdk/backend/python/tests`.