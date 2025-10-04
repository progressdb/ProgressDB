
# Tests: Feature-Oriented Objectives and Flows

This document defines the **"tests per feature"** system and the objectives for server E2E tests. We adopt a single-tier E2E approach: each server test starts a real server process, configures it for the scenario, exercises the feature via the HTTP surface, then tears the server down. There are no unit/in-process/integration subsystems for server tests — every feature test is a real-process E2E test and contains all scenarios for that feature in a single file.

---

## Principles

- **One feature → one feature test file:** All scenarios related to that feature live in a single test file (e.g. `authentication_test.go`) and that file fully exercises the feature's behavior across configurations and edge cases.
- **Real-process only:** Tests build and spawn the server binary per-test, using per-test configs, DB paths, ports and keys. Tests exercise the exact runtime behaviour of the shipped binary.
- **Isolation:** Each test uses a temporary working directory, a unique DB path, and deterministic test keys; tests run serially by default.
- **Determinism:** Tests should avoid blind sleeps; use readiness probes and polling-with-timeout. Prefer explicit test-trigger admin endpoints for background jobs (e.g. retention) so tests are deterministic.
- **Diagnostics:** On failure each test must capture and emit server stdout/stderr and any relevant DB files for post-mortem.

---

## Test Flow (Per-Test)

1. **Build or locate** the server binary to test.
2. **Create a temporary workdir** and write a minimal YAML config tailored to the scenario (db path, api keys, retention settings, kms master key, etc.).
3. **Start the server** as a child process, redirecting stdout/stderr to files in the tempdir.
4. **Wait for readiness** (`/healthz`) with a timeout; on failure dump logs and fail the test.
5. **Exercise the feature** via HTTP calls, direct DB access (if needed), or admin trigger endpoints.
6. **Use polling with timeouts** to assert eventual state changes (e.g. purge completed).
7. **Stop the server, collect logs and artifacts, and remove temp files.**

---

## Helper Guidance

- Implement a reusable `StartServerProcess` helper to build, spawn, wait-for-ready, stop, and collect logs.
- Tests should write per-test configs (YAML) and pass them via `PROGRESSDB_SERVER_CONFIG` or a command-line flag.
- Use a deterministic embedded KMS provider or pass a test master key via config.
- Prefer selecting free ports deterministically and run tests serially to avoid conflicts.

---

## Global Policy

- All server tests are E2E real-process tests. There are no separate in-process integration tests for the server — the in-process helper `SetupServer()` may still be useful for very fast package-level tests, but production-facing feature tests must use the real server process.

---

## Objectives Per Feature

### 1. Logging System — `logging_test.go`

**Objectives:**
1. Start server with default logging configuration — verify startup banner and basic Info logging to stdout.
2. Start server with an audit sink configured — verify an audit file is created and audit JSON lines are emitted for admin/audit events.
3. Validate log level behavior (DEBUG/INFO/WARN/ERROR) via configuration.
4. Validate failure modes (e.g., audit path permission error) surface during startup or fall back gracefully per current behaviour.
5. Smoke test concurrent logging during load and verify no panics and logs are produced.

---

### 2. Configs System — `configs_test.go`

**Objectives:**
1. Start server with malformed global config — expect startup to fail fast with a user-friendly error.
2. Start server with malformed per-feature config and verify startup fails fast for that feature.
3. Start server with selective feature toggles (on/off) and verify correct startup and exposed endpoints.
4. Start server with all features enabled and verify the full surface is available.

---

### 3. Authentication System — `authentication_test.go`

**Objectives:**
1. Start server with no API keys and verify request handling for anonymous/signed requests.
2. Start server with frontend/backend/admin API keys (configurable in the test) and assert scope enforcement for each role.
3. Validate signing flows: ensure signing keys (derived from backend keys) are used to verify `X-User-Signature` and that misconfiguration yields clear failures.

**Test Scope Inside Single File:**
- The single `authentication_test.go` file contains all scenarios: no keys, frontend-only, backend-only, admin-only, and combinations. Each test scenario starts its own server process with a tailored config and cleans up.

---

### 4. Authorization System — `authorization_test.go`

**Objectives:**
1. Validate CORS configuration affects allowed origins as configured.
2. Validate rate limiting per-config (RPS/Burst) and ensure limits are enforced.
3. Validate role-based resource visibility (e.g., soft-deleted threads visible to admins only).
4. Validate author/ownership checks and header/signature tampering protections.

---

### 5. Handlers System — `handlers_test.go`

**Objectives:**
1. End-to-end verification of handler behaviors for valid and invalid inputs.
2. Verify CRUD flows for threads/messages and edge cases (pagination, filtering, validation failures).
3. Ensure handlers integrate correctly with store and encryption settings.

---

### 6. Encryption System — `encryption_test.go`

**Objectives:**
1. Verify DEK provisioning on thread creation when encryption is enabled.
2. Verify encryption/decryption round-trips: API returns plaintext while DB stores ciphertext.
3. Verify DEK rotation keeps messages decryptable.
4. Verify KMS/mk validation and fail-fast behavior for missing master keys.

---

### 7. Retention System — `retention_test.go`

**Objectives:**
1. Verify file-lease Acquire/Renew/Release semantics.
2. Verify purge behavior: soft-delete + purge window leads to permanent deletion.
3. Verify dry-run/audit modes write audit records without deleting data.
4. Tests should trigger retention deterministically (test trigger endpoint) rather than relying on timers.

---

### 8. Utils System — `utils_test.go`

**Objectives:**
1. Validate utility helpers: ID generation, slug generation, path splitting, JSON helpers.
2. Cover boundary and error cases for helper functions.

---

## Cross-Feature Integration Tests

- Keep a small set of cross-feature scenarios in `integration_test.go` that exercise end-to-end workflows combining multiple features (e.g., encryption + retention + handlers). These are still real-process tests and should be used sparingly to validate complex interactions.

---

## CI & Running

- E2E tests are heavier and slower. Run them in CI on protected branches, nightly pipelines, or on-demand.
- Use `scripts/test-e2e.sh` to run the full suite locally or in CI; tests are serial by default to avoid port/DB conflicts.

---

## Diagnostics

- On failure, tests must capture and print server stdout/stderr and attach temp DB files to CI artifacts for debugging.

---

## Implementation Notes

- Implement `StartServerProcess` helper in `server/tests/e2ehelpers` that builds, starts and stops the binary, waits for readiness, and returns log paths.
- Tests must write per-test YAML configs and point the server to them via env or flags.