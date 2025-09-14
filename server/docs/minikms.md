# miniKMS (ProgressDB) — design & runbook

This document describes the miniKMS implementation shipped with ProgressDB, how it is wired into the server, operational notes, and recommended hardening and runbook steps.

## Overview

- Purpose: provide a local, hardened key service used to manage master KEK and wrap per-thread DEKs. It keeps the high-value KEK out of the main server process and performs wrap/unwrap + encryption operations.
- Process model: miniKMS runs as a separate process (spawned by the server by default). The server uses a small RemoteClient over a Unix Domain Socket (UDS) to call the KMS.
- Threat model: protects against accidental key leakage and reduces blast radius of server compromises. It is NOT a replacement for HSM/cloud KMS for the highest assurance levels.

## Components

- `server/cmd/minikms` — the miniKMS HTTP server (HTTP over UDS). Exposes admin and crypto endpoints and persists wrapped DEKs and metadata in the configured data directory.
- `server/pkg/kms/remote_client.go` — client adapter used by the server to talk to miniKMS over the UDS.
- `server/pkg/kms/local.go` — embedded dev provider (kept for local testing only).
- `server/pkg/security` — pluggable security bridge. The server calls `security.CreateDEKForThread`, `security.EncryptWithKey`, `security.DecryptWithKey` to interact with the provider.
- `server/pkg/kms/starter.go` — helper the server uses to spawn and supervise the miniKMS child.
- `server/pkg/store` — stores wrapped DEK metadata, thread->key mapping, and messages.

## Runtime flow (normal)

1. On startup the server spawns miniKMS (or you may run it externally). The server passes allowed peer UIDs to the child (so miniKMS can accept UDS peer credentials).
2. Server registers `RemoteClient` pointing at the child UDS socket and delegates KMS operations to it.
3. When storing a message, the server asks for the thread DEK (`CreateDEKForThread` if missing) and calls `EncryptWithKey(keyID, plaintext)` which is executed inside miniKMS. The server never holds raw DEKs.
4. When reading, the server calls `DecryptWithKey(keyID, ciphertext)` and receives plaintext from miniKMS.

## Authentication & Authorization

- Primary (recommended): UDS peer-credential (SO_PEERCRED on Linux, getpeereid on BSD/macOS). miniKMS accepts requests only from configured peer UIDs (env `PROGRESSDB_MINIKMS_ALLOWED_UIDS`).
- Policy: the server sets `PROGRESSDB_MINIKMS_ALLOWED_UIDS` to its UID when spawning the child, so local server→miniKMS calls are allowed without secrets in memory.
- API-key fallback: removed for the default local spawn flow. (If you need remote KMS, you must add a secure auth method such as mTLS or short-lived tokens.)

## API (internal HTTP over UDS)

- POST `/create_dek_for_thread` { "thread_id": "..." } → { "key_id": "...", "wrapped": "<base64>" }
- GET `/get_wrapped?key_id=...` → { "wrapped": "<base64>" }
- POST `/encrypt` { "key_id": "...", "plaintext": "<base64>" } → { "ciphertext": "<base64>" }
- POST `/decrypt` { "key_id": "...", "ciphertext": "<base64>" } → { "plaintext": "<base64>" }
- POST `/rotate_kek` { "new_kek_hex": "..." } — admin endpoint to rewrap all DEKs under a new KEK.

Notes: these endpoints are internal and are intended to be called by the server process over the UDS. They require peer-cred auth.

## Rotation

- KEK rotation: miniKMS exposes `/rotate_kek` which accepts a new KEK (hex). The handler:
  - reads all persisted wrapped-DEK metadata, unwraps each DEK with the current KEK, rewraps it with the new KEK, writes updated metadata and increments version, and keeps a backup of old metadata under `kms-deks-backup/`.
  - after successful rewrap of all DEKs, swaps the active KEK to the new value.
- DEK rotation (per-thread): the server exposes an admin endpoint `/admin/rotate_thread_dek` which:
  - creates a new DEK for the thread and calls a migration routine that decrypts each message with the old DEK and encrypts it with the new DEK, backing up old ciphertexts before overwrite. This is a blocking migration; for production you should run it as a resumable background job (see next steps).

## Persistence & backups

- Wrapped DEKs: stored in `<data-dir>/kms-deks/<keyid>.json` (JSON KeyMeta with base64 wrapped blob). Backup created by rotate_kek and backup writes under `<data-dir>/kms-deks-backup/`.
- Thread->key mapping: stored in Pebble under `kms:map:thread:<threadID>`.
- Messages: stored under `thread:<threadID>:msg:<timestamp>-<seq>` as either ciphertext (nonce|ct) when encrypted.

## Audit

- miniKMS appends signed audit lines to `<data-dir>/minikms-audit.log`. Each line: { "event": <json>, "sig": "<base64-hmac>" }.
- Current signing: HMAC-SHA256 using the KEK (short-term). For better assurance, use a separate signing key (HSM/cloud KMS) and forward audit to a secure collector.

## Memory hardening

- `mlock` (where available) is used to lock KEK bytes and cached DEKs in RAM and reduce swap exposure. Cached DEKs are zeroized on eviction and on provider close. Document and set `LimitMEMLOCK` for the miniKMS service user.

## Runbook (quick)

- Start miniKMS (server will spawn by default): ensure miniKMS binary is present or set `PROGRESSDB_KMS_BINARY`.
- Recommended systemd unit for miniKMS (example for operations):

```
[Unit]
Description=ProgressDB miniKMS

[Service]
User=minikms
Group=minikms
ExecStart=/opt/progressdb/minikms --socket /var/run/progressdb-minikms.sock --data-dir /var/lib/progressdb/minikms
LimitMEMLOCK=infinity
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=full

[Install]
WantedBy=multi-user.target
```

- Ensure socket directory ownership and permissions: `chown minikms:minikms /var/run && chmod 0700 /var/run` and the socket file will be 0600.
- Backup KEKs/metadata before rotation; rotate KEK during low-activity windows and verify decrypts of sample messages.

## Tests & validation

- Add unit tests for encrypt/decrypt, CreateDEKForThread, and KEK rotation. Add an integration test that spawns miniKMS (UDS) and performs encrypt->decrypt and rotate_kek.

## Next steps (recommended)

1. Move audit signing key to a separate signing key (HSM or cloud KMS) and forward events to a remote SIEM.
2. Implement resumable, checkpointed DEK migration for large threads (run as background job). The current `RotateThreadDEK` is synchronous and should be made resumable.
3. Add stronger auth for networked setups: mTLS or OIDC token flow; do not load long-lived client private keys into the server process.
4. Add operational examples (k8s sidecar, docker-compose, and systemd) and a detailed runbook (unseal, rotate, recover).

---

This is the authoritative miniKMS reference for ProgressDB. If you want, I can now:
- add a resumable migration worker (background job) and an admin CLI to run/monitor it, or
- add tests and CI for rotation and migration, or
- add the systemd/k8s example files and a more detailed runbook.

