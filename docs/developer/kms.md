# KMS — In-depth Developer Guide

This document explains the current KMS design used by the server: architecture, provider interface, HTTP protocol for remote KMS, embedded provider behavior, DEK lifecycle, storage schema, rotation, audit, and developer/testing tips.

1. Architecture
---------------
- Purpose: manage master KEK, create/unwrap per-thread DEKs, and perform encrypt/decrypt operations so the server does not handle raw DEKs.
- Modes:
  - External KMS: separate binary `progressdb-kms` that exposes HTTP endpoints (over Unix Domain Socket or TCP). Recommended for production.
  - Embedded/local provider: in-process provider used for development and tests (server holds master key in memory).
- The server interacts with KMS through a pluggable provider interface (`server/pkg/kms` + `server/pkg/security` bridge). The remote client adapts provider calls to HTTP endpoints.

2. Provider interface (server-side)
----------------------------------
The server expects a provider implementing a small interface in `server/pkg/kms`:
- `CreateDEKForThread(threadID) (keyID string, wrapped []byte, kekID string, kekVersion string, err error)`
- `EncryptWithDEK(dekID string, plaintext, aad []byte) (ciphertext []byte, keyVersion string, err error)`
- `DecryptWithDEK(dekID string, ciphertext, aad []byte) (plaintext []byte, err error)`
- Generic variants: `CreateDEK()`, `Encrypt`, `Decrypt`, `WrapDEK`, `UnwrapDEK` for embedded/local usage.

3. Remote client (HTTP) protocol
--------------------------------
The `RemoteClient` translates provider calls into HTTP requests against the KMS process. Conventions:
- Request/response bodies use JSON; binary blobs (wrapped DEKs, ciphertext, plaintext) are base64-encoded in JSON.
- Endpoints implemented by the local KMS server:
  - POST `/create_dek_for_thread` { "thread_id": "<id>" } -> { "key_id": "k_<..>", "wrapped": "<base64>" }
  - POST `/encrypt` { "key_id": "k_..", "plaintext": "<base64>" } -> { "ciphertext": "<base64>" }
  - POST `/decrypt` { "key_id": "k_..", "ciphertext": "<base64>" } -> { "plaintext": "<base64>" }
  - GET  `/get_wrapped?key_id=...` -> { "wrapped": "<base64>" }

Implementation notes:
- The remote client uses an `http.Client` with a custom transport that dials the unix socket or TCP endpoint using `DialContext` so requests respect context timeouts and cancellation.
- Client decodes base64 and returns raw []byte to caller code.

4. Embedded provider
---------------------
- For dev/test, the server may register an in-process provider that holds a master key (AES-256) and performs encryption/decryption locally.
- The embedded provider implements the same interface but may return wrapped DEKs and keep unwrapped DEKs in memory for short-term caching.
- This mode is useful for local development; in production prefer an external KMS.

5. Storage & canonical metadata
--------------------------------
- Thread metadata (`thread:<threadID>:meta`) contains the canonical KMS information in `kms` fields:
  - `kms.key_id` — provider key handle used to encrypt/decrypt child messages
  - `kms.wrapped_dek` — optional base64-wrapped DEK value
  - `kms.kek_id`, `kms.kek_version` — optional KEK metadata
- Messages store only ciphertext (commonly nonce|ciphertext). No per-message DEK info is stored.

6. DEK lifecycle
-----------------
 - Provisioning: per-thread DEK is created at thread creation time. The server calls `CreateDEKForThread` and persists `kms.key_id` into the thread meta.
 - Encryption: messages are encrypted by calling `EncryptWithDEK(keyID, plaintext)` with the thread keyID.
 - Decryption: messages are decrypted using `DecryptWithDEK(keyID, ciphertext)`.

7. Rotation & rewrap
---------------------
- KEK rotation (rewrap all DEKs under new KEK) is performed inside KMS (admin operation) and requires backups of wrapped DEKs.
- Per-thread DEK rotation (creating a new DEK and migrating message ciphertexts) can be performed via admin APIs (server performs decrypt->encrypt migration). Prefer offline or background migration.

8. Security considerations
--------------------------
- Never log KEKs, wrapped DEKs, or raw DEKs. Log only key identifiers.
- Protect the external KMS endpoint:
  - For UDS: restrict filesystem permissions and use peer-credential checks.
  - For TCP: use mTLS and firewall rules.
- Use strict permissions for any generated KMS config that contains `master_key_hex` (0600 and owned by service user).

9. Dev & testing tips
---------------------
- Embedded mode is easiest for tests: set server to use in-process provider (masters in-memory).
- For end-to-end integration: start a KMS process locally and point `PROGRESSDB_KMS_ENDPOINT` at it (UDS path or localhost:port).
- Example dev flow:
  1. Start KMS (embedded or external)
  2. Start server with encryption enabled
  3. Create thread (server provisions DEK on thread)
  4. Post messages and fetch them (verify decrypt)

10. Code pointers
------------------
- Server provider bridge: `server/pkg/security/crypto.go`
- Remote client implementation: `server/pkg/kms/external.go`
- Embedded provider adapter: `server/pkg/kms/embedded.go` and `kms/pkg/security/hashicorp_provider.go`
- Thread message storage and decrypt logic: `server/pkg/store/pebble.go`
- Thread creation and DEK provisioning: `server/pkg/api/handlers/threads.go` (createThread)

# 11. Troubleshooting
--------------------
- Decrypt fails: check thread meta (`thread:<id>:meta`) contains `kms.key_id` and KMS endpoint is healthy.
- Provider errors: check remote client logs and KMS logs; enable debug logging on the remote client for HTTP responses.
- Rotation: always back up wrapped DEKs before rewrap and verify sample decrypts.

# 12. KMS Storage Layer: Deep Dive
--------------------------------

This section explains how KMS stores DEKs and metadata now that per-thread DEKs are managed by KMS and thread metadata is canonical.

High-level summary
- KMS is authoritative for DEK material; the server stores only references (`kms.key_id`) in thread metadata and ciphertext in the server DB.

What KMS stores
- Wrapped DEKs: provider-wrapped DEK blobs (binary, typically stored base64 in JSON over HTTP).
- DEK metadata: key_id, created_at, kek_id, kek_version, audit fields.
- Audit logs and rotation backups: separate files under the KMS data directory.

Where it lives
- KMS on-disk DB (Pebble) under `<data_dir>/kms.db` stores `dek:<keyID>` and `dekmeta:<keyID>` entries.
- Server Pebble stores thread metadata at `thread:<threadID>:meta` (contains `kms.key_id`) and message ciphertext at `thread:<threadID>:msg:<ts>-<seq>`.

Flows
- Provision DEK at thread creation: server calls `CreateDEKForThread(threadID)` and persists `kms.key_id` into the thread meta.
- Encrypt message: server calls `EncryptWithDEK(keyID, plaintext)` (KMS returns nonce|ciphertext) and stores ciphertext in server DB.
- Decrypt message: server calls `DecryptWithDEK(keyID, ciphertext)` and receives plaintext.

Rotation & backups
- KEK rotation is carried out inside KMS: unwrap all wrapped DEKs with old KEK, rewrap with new KEK, persist backups before overwriting.

Consistency & concurrency
- Because DEKs are provisioned at thread creation, readers will find `kms.key_id` present when messages exist.
- If you later support first-writer provisioning, use per-thread locks + atomic DB batch to avoid races.

Security & operational notes
- Do not store raw DEKs. Store only wrapped blobs and key identifiers.
- Protect master KEK and KMS endpoint with filesystem permissions, mTLS, or firewall rules.

Developer & testing notes
- Embedded provider is handy for tests. For end-to-end tests, run a KMS process locally and point `PROGRESSDB_KMS_ENDPOINT` at it.
