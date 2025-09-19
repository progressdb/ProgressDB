# KMS (ProgressDB) — design & runbook

This document describes the KMS implementation shipped with ProgressDB, how it is wired into the server, operational notes, and recommended hardening and runbook steps.

## Overview

- Purpose: provide a local, hardened key service used to manage master KEK and wrap per-thread DEKs. It keeps the high-value KEK out of the main server process and performs wrap/unwrap + encryption operations.
 - Process model: KMS runs as a separate process (spawned by the server by default). The server uses a small RemoteClient over HTTP/TCP to call the KMS.
- Threat model: protects against accidental key leakage and reduces blast radius of server compromises. It is NOT a replacement for HSM/cloud KMS for the highest assurance levels.

## Components

 - `kms` — the KMS HTTP server (separate project/binary; HTTP over TCP). Exposes admin and crypto endpoints and persists wrapped DEKs and metadata in the configured data directory.
 - `server/pkg/kms/external.go` — client adapter used by the server to talk to KMS over HTTP/TCP.
 - The server no longer auto-spawns a KMS child by default; it can run in
  embedded mode (in-process) or talk to an external `progressdb-kms` process via the
  remote client.
 - `server/pkg/security` — pluggable security bridge. The server calls `security.CreateDEKForThread`, `security.EncryptWithDEK`, `security.DecryptWithDEK` to interact with the provider.
- `server/pkg/store` — stores wrapped DEK metadata, thread->key mapping, and messages.

## Runtime flow (normal)

1. On startup the server either initializes an embedded KMS provider (in-process) or connects to an external `progressdb-kms` process. Communication with an external KMS uses HTTP over TCP.
2. In external mode the server constructs a `RemoteClient` pointing at the configured endpoint and delegates KMS operations to it.
3. When storing a message, the server asks for the thread DEK (`CreateDEKForThread` if missing) and calls `EncryptWithDEK(keyID, plaintext)` which is executed inside KMS. The server never holds raw DEKs.
4. When reading, the server calls `DecryptWithDEK(keyID, ciphertext)` and receives plaintext from KMS.

## Authentication & Authorization

 - Primary (recommended): secure the KMS HTTP endpoint with network-level protections or transport auth (mTLS, tokens). The KMS records caller information in audit logs; caller authorization should be enforced by the supervising environment.
- API-key fallback: removed for the default local spawn flow. (If you need remote KMS, you must add a secure auth method such as mTLS or short-lived tokens.)

## API (internal HTTP)

- POST `/create_dek_for_thread` { "thread_id": "..." } → { "key_id": "...", "wrapped": "<base64>" }
- GET `/get_wrapped?key_id=...` → { "wrapped": "<base64>" }
- POST `/encrypt` { "key_id": "...", "plaintext": "<base64>" } → { "ciphertext": "<base64>" }
- POST `/decrypt` { "key_id": "...", "ciphertext": "<base64>" } → { "plaintext": "<base64>" }
- POST `/rotate_kek` { "new_kek_hex": "..." } — admin endpoint to rewrap all DEKs under a new KEK.

Notes: these endpoints are internal and are intended to be called by the server process over HTTP/TCP. Protect the endpoint via mTLS, tokens or network controls.

## Rotation

- KEK rotation: KMS exposes `/rotate_kek` which accepts a new KEK (hex). The handler:
  - iterates all stored wrapped-DEK metadata in the metadata store (Pebble DB), unwraps each DEK with the current KEK, rewraps it with the new KEK, updates its metadata (version/kek id), and keeps a file backup snapshot per-key under `kms-deks-backup/` for recovery.
  - after successful rewrap of all DEKs, swaps the active KEK to the new value.

Configuration and startup secrets

- KMS accepts an optional `--config <path>` parameter that points to a YAML file. When supplied KMS will read the master key file path and the allowed peer UIDs from this file and will not use environment variables for those startup secrets. This is the recommended secure operating mode.

Example `config.yaml` (self-contained):

```yaml
master_key_hex: "<64-hex-bytes>"
```

Convenience: the server exposes an admin endpoint to generate a compatible
master KEK for local use. POST `/admin/encryption/generate-kek` (admin role required)
returns JSON `{ "kek_hex": "<64-hex-bytes>" }`. This is intended for
operators who need a quick way to produce a properly-sized KEK; prefer
provisioning secrets via orchestrator mechanisms or a secure secret store
in production.
- DEK rotation (per-thread): the server exposes an admin endpoint `/admin/encryption/rotate-thread-dek` which:
  - creates a new DEK for the thread and calls a migration routine that decrypts each message with the old DEK and encrypts it with the new DEK, backing up old ciphertexts before overwrite. This is a blocking migration; for production you should run it as a resumable background job (see next steps).

## Persistence & backups

- Wrapped DEKs and their metadata: stored in a compact Pebble DB file at `<data-dir>/kms.db` (prefixed keys, e.g. `meta:<keyid>`). Backups for rotate/rewrap operations are written as individual files under `<data-dir>/kms-deks-backup/` to preserve a human-readable history and allow ad-hoc recovery.
 - Thread->key mapping: stored in thread metadata under `thread:<threadID>:meta` (field `kms.key_id`).
- Messages: stored under `thread:<threadID>:msg:<timestamp>-<seq>` as either ciphertext (nonce|ct) when encrypted.

## Audit

- KMS appends signed audit lines to `<data-dir>/kms-audit.log`. Each line: { "event": <json>, "sig": "<base64-hmac>" }.
- Current signing: HMAC-SHA256 using the KEK (short-term). For better assurance, use a separate signing key (HSM/cloud KMS) and forward audit to a secure collector.

## Memory hardening

- `mlock` (where available) is used to lock KEK bytes and cached DEKs in RAM and reduce swap exposure. Cached DEKs are zeroized on eviction and on provider close. Document and set `LimitMEMLOCK` for the KMS service user.

## Runbook (quick)

-- Start KMS (external mode): ensure a `progressdb-kms` binary is present on `PATH` or installed on the host and start it separately when using `PROGRESSDB_KMS_MODE=external`.
- Recommended systemd unit for the `progressdb-kms` service (example):

```
[Unit]
Description=ProgressDB KMS
After=network.target

[Service]
Type=simple
User=kms
Group=kms
ExecStart=/usr/local/bin/progressdb-kms --endpoint 127.0.0.1:6820 --data-dir /var/lib/progressdb/kms --config /etc/progressdb/kms-config.yaml
LimitMEMLOCK=infinity
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=full

[Install]
WantedBy=multi-user.target
```

- Ensure runtime directories ownership and permissions: `chown kms:kms /var/run && chmod 0700 /var/run`. When using a TCP endpoint, ensure network-level access controls are configured (firewall, mTLS, etc.).
- Backup KEKs/metadata before rotation; rotate KEK during low-activity windows and verify decrypts of sample messages.

## Tests & validation

 - Add unit tests for encrypt/decrypt, CreateDEKForThread, and KEK rotation. Add an integration test that spawns KMS (HTTP) and performs encrypt->decrypt and rotate_kek.

---

This is the authoritative KMS reference for ProgressDB.
