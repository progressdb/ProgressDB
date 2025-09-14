# KMS (ProgressDB) — design & runbook

This document describes the KMS implementation shipped with ProgressDB, how it is wired into the server, operational notes, and recommended hardening and runbook steps.

## Overview

- Purpose: provide a local, hardened key service used to manage master KEK and wrap per-thread DEKs. It keeps the high-value KEK out of the main server process and performs wrap/unwrap + encryption operations.
- Process model: KMS runs as a separate process (spawned by the server by default). The server uses a small RemoteClient over a Unix Domain Socket (UDS) to call the KMS.
- Threat model: protects against accidental key leakage and reduces blast radius of server compromises. It is NOT a replacement for HSM/cloud KMS for the highest assurance levels.

## Components

- `kms` — the KMS HTTP server (separate project/binary; HTTP over UDS). Exposes admin and crypto endpoints and persists wrapped DEKs and metadata in the configured data directory.
- `server/pkg/kms/remote_client.go` — client adapter used by the server to talk to KMS over the UDS.
- `server/pkg/kms/starter.go` — helper the server uses to spawn and supervise the KMS child.
- `server/pkg/security` — pluggable security bridge. The server calls `security.CreateDEKForThread`, `security.EncryptWithKey`, `security.DecryptWithKey` to interact with the provider.
- `server/pkg/store` — stores wrapped DEK metadata, thread->key mapping, and messages.

## Runtime flow (normal)

1. On startup the server spawns KMS (or you may run it externally). The server passes allowed peer UIDs to the child (so KMS can accept UDS peer credentials).
2. Server registers `RemoteClient` pointing at the child UDS socket and delegates KMS operations to it.
3. When storing a message, the server asks for the thread DEK (`CreateDEKForThread` if missing) and calls `EncryptWithKey(keyID, plaintext)` which is executed inside KMS. The server never holds raw DEKs.
4. When reading, the server calls `DecryptWithKey(keyID, ciphertext)` and receives plaintext from KMS.

## Authentication & Authorization

- Primary (recommended): UDS peer-credential (SO_PEERCRED on Linux, getpeereid on BSD/macOS). KMS accepts requests only from configured peer UIDs (env `PROGRESSDB_KMS_ALLOWED_UIDS`).
- Policy: the server sets `PROGRESSDB_KMS_ALLOWED_UIDS` to its UID when spawning the child, so local server→KMS calls are allowed without secrets in memory.
- API-key fallback: removed for the default local spawn flow. (If you need remote KMS, you must add a secure auth method such as mTLS or short-lived tokens.)

## API (internal HTTP over UDS)

- POST `/create_dek_for_thread` { "thread_id": "..." } → { "key_id": "...", "wrapped": "<base64>" }
- GET `/get_wrapped?key_id=...` → { "wrapped": "<base64>" }
- POST `/encrypt` { "key_id": "...", "plaintext": "<base64>" } → { "ciphertext": "<base64>" }
- POST `/decrypt` { "key_id": "...", "ciphertext": "<base64>" } → { "plaintext": "<base64>" }
- POST `/rotate_kek` { "new_kek_hex": "..." } — admin endpoint to rewrap all DEKs under a new KEK.

Notes: these endpoints are internal and are intended to be called by the server process over the UDS. They require peer-cred auth.

## Rotation

- KEK rotation: KMS exposes `/rotate_kek` which accepts a new KEK (hex). The handler:
  - reads all persisted wrapped-DEK metadata, unwraps each DEK with the current KEK, rewraps it with the new KEK, writes updated metadata and increments version, and keeps a backup of old metadata under `kms-deks-backup/`.
  - after successful rewrap of all DEKs, swaps the active KEK to the new value.
- DEK rotation (per-thread): the server exposes an admin endpoint `/admin/rotate_thread_dek` which:
  - creates a new DEK for the thread and calls a migration routine that decrypts each message with the old DEK and encrypts it with the new DEK, backing up old ciphertexts before overwrite. This is a blocking migration; for production you should run it as a resumable background job (see next steps).

## Persistence & backups

- Wrapped DEKs: stored in `<data-dir>/kms-deks/<keyid>.json` (JSON KeyMeta with base64 wrapped blob). Backup created by rotate_kek and backup writes under `<data-dir>/kms-deks-backup/`.
- Thread->key mapping: stored in Pebble under `kms:map:thread:<threadID>`.
- Messages: stored under `thread:<threadID>:msg:<timestamp>-<seq>` as either ciphertext (nonce|ct) when encrypted.

## Audit

- KMS appends signed audit lines to `<data-dir>/kms-audit.log`. Each line: { "event": <json>, "sig": "<base64-hmac>" }.
- Current signing: HMAC-SHA256 using the KEK (short-term). For better assurance, use a separate signing key (HSM/cloud KMS) and forward audit to a secure collector.

## Memory hardening

- `mlock` (where available) is used to lock KEK bytes and cached DEKs in RAM and reduce swap exposure. Cached DEKs are zeroized on eviction and on provider close. Document and set `LimitMEMLOCK` for the KMS service user.

## Runbook (quick)

- Start KMS (server will spawn by default): ensure KMS binary is present or set `PROGRESSDB_KMS_BINARY`.
- Recommended systemd unit for KMS (example for operations):

```
[Unit]
Description=ProgressDB KMS

[Service]
User=kms
Group=kms
ExecStart=/usr/local/bin/kms --socket /var/run/progressdb-kms.sock --data-dir /var/lib/progressdb/kms
LimitMEMLOCK=infinity
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=full

[Install]
WantedBy=multi-user.target
```

- Ensure socket directory ownership and permissions: `chown kms:kms /var/run && chmod 0700 /var/run` and the socket file will be 0600.
- Backup KEKs/metadata before rotation; rotate KEK during low-activity windows and verify decrypts of sample messages.

## Tests & validation

- Add unit tests for encrypt/decrypt, CreateDEKForThread, and KEK rotation. Add an integration test that spawns KMS (UDS) and performs encrypt->decrypt and rotate_kek.

---

This is the authoritative KMS reference for ProgressDB.

