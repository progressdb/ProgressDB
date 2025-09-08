# Changelog

All notable changes to this project are documented in this file.

The format is based on "Keep a Changelog" and releases are tagged with `vMAJOR.MINOR.PATCH`.

## [Unreleased]

- (work in progress)

## [0.1.0] - 2025-09-07

### Added

- Server: structured logging and startup banner
- Server: Prometheus metrics endpoint (`/metrics`) and basic telemetry
- Server: configuration via flags, env and config file (`config` package)
- Server: security middleware (CORS, API key handling, TLS support)
- Server: rate limiting hooks and configuration (RPS / burst)
- Messages: append-only storage, versioning of messages, soft-delete (tombstones), reply-to support, reactions map
- Threads: CRUD for thread metadata and indexable thread storage
- Auth: backend/admin API key handling and signing helper primitives
- Data viewer: static admin viewer served at `/viewer/`

### SDKs

- Backend SDKs: Node.js backend SDK (`clients/sdk/backend/nodejs`) present
- Backend SDKs: Python backend SDK (`clients/sdk/backend/python`) present and packaged
- Frontend SDKs: TypeScript core (`@progressdb/js`) and React bindings (`@progressdb/react`) present

### Dev & CI tooling

- Added `scripts/build.sh` and `scripts/release.sh` to build single binaries and multi-arch releases
- Added goreleaser config (`.goreleaser.yml`) and GitHub Actions workflow (`.github/workflows/release.yml`) to automate release builds and GitHub Releases
- Added helper publish scripts for SDKs under `scripts/sdk` and README docs for releases (`RELEASE.md`, `.github/README.md`)

### Documentation

- README updated with quickstart examples and SDK usage snippets
- Roadmap (`ROADMAP.md`) and release guide added/updated

