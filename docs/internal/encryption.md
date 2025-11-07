# Encryption & KMS — Developer Guide

This document explains the current server KMS/encryption design, runtime flows, and operational notes. It describes the canonical places for DEK metadata and how messages are encrypted and decrypted.

1) Architecture
-----------------
- Purpose: keep raw DEKs out of the main server process, provision per-thread DEKs, and perform encryption/decryption via a KMS provider.
- Modes:
  - External KMS: separate `prgkms` process; server talks to it via HTTP (UDS or TCP).
  - Embedded/dev provider: in-process provider used for development or tests (master key in memory).

2) Canonical storage
---------------------
- Thread metadata (`thread:<threadID>:meta`) contains the canonical KMS info in a `kms` field:
  - `kms.key_id` (string): the provider key identifier for the thread DEK.
  - `kms.wrapped_dek` (optional base64): wrapped DEK if provider returns it and server stores it.
  - `kms.kek_id`, `kms.kek_version` (optional): KEK metadata used for rotation/audit.
- Messages store only ciphertext (nonce|ciphertext). They do not store per-message DEK info.

3) Core server–KMS flows
-------------------------
- Thread creation (recommended workflow):
  1. Server creates thread metadata (POST /v1/threads).
  2. If encryption is enabled, server calls `kms.CreateDEKForThread(threadID)` and writes `kms.key_id` into the thread metadata before returning.
- Writing a message:
  1. Server reads `thread.<id>.meta.kms.key_id`.
  3. Stores the ciphertext under `thread:<threadID>:msg:<ts>-<seq>`.
- Reading messages:
  1. Server reads the thread metadata to obtain `kms.key_id`.
  2. If the provider is unavailable, and an embedded master key is configured, the server may fall back to local master-key decryption (server-side encryption helpers).

4) Provider interface the server expects
----------------------------------------
 - CreateDEKForThread(threadID) -> (keyID, wrappedDEK, kekID, kekVersion, error)
 - Calls `kms.EncryptWithDEK(keyID, plaintext)` which delegates to provider/KMS.

 - For each ciphertext, server calls `kms.DecryptWithDEK(keyID, ciphertext)` to get plaintext.

- EncryptWithDEK(dekID string, plaintext, aad []byte) (ciphertext []byte, keyVersion string, err error)
- DecryptWithDEK(dekID string, ciphertext, aad []byte) (plaintext []byte, err error)
- Fallbacks: CreateDEK, Encrypt, Decrypt used by embedded provider.

5) KMS HTTP endpoints (remote client mapping)
----------------------------------------------
- POST /create_dek_for_thread { thread_id } -> { key_id, wrapped }
- POST /encrypt { key_id, plaintext(base64) } -> { ciphertext(base64) }
- POST /decrypt { key_id, ciphertext(base64) } -> { plaintext(base64) }
- GET /get_wrapped?key_id=... -> { wrapped: base64 }

6) DEK lifecycle & rotation
----------------------------
- DEK created per-thread and referenced by `kms.key_id`.
- KEK rotation: rewrap DEKs under a new KEK inside KMS; provider updates kek metadata.
- DEK rotation for a thread: admin flow creates a new DEK and migrates messages (optional background job).

7) Security & secrets
----------------------
- Master KEK must be provided securely (config file with strict permissions or a secret store).
- Do not log raw KEKs or wrapped raw keys.
- Protect external KMS endpoints via UDS perms, mTLS, or firewall rules.

8) Development & local usage
----------------------------
- `scripts/dev.sh` supports encrypted runs in embedded or external mode.
- Dev helpers create ephemeral master keys and local KMS configs for convenience.
- Workflow: create thread (server provisions DEK) → post messages → read messages.

9) Operational notes
---------------------
- Threads must be created before posting messages when encryption is enabled (server provisions DEK at creation).
- We removed legacy `kms:map:thread*` keyspace; thread metadata is canonical.
- If you need first-writer provisioning later, add per-thread locks and atomic DB batch write for mapping+first message.

10) Troubleshooting
-------------------
- Decrypt failures: verify thread meta contains `kms.key_id` and KMS provider is healthy.
- Missing mapping: ensure thread was created with DEK provisioned; check admin endpoints for key info.
- Rotation issues: back up wrapped DEKs and validate decrypts after rewrap.
