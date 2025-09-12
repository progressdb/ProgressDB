# Encryption

This document describes the optional AES‑256‑GCM encryption features in ProgressDB: how to configure, what is encrypted, and operational considerations.


1. you provide the encryption key
2. on service startup - a minihsm is started on a different process (with the master key)
3. - right after, the main process key is destroyed
4. the minihsm takes threadkeys and encryptes or decrypts them for thread message decryptions.
5. for full hsm - use a kms service.


Overview
--------
- ProgressDB supports server-side AES‑256‑GCM encryption for stored messages.
- Two modes are supported:
  - **Field-level (selective) JSON path encryption**: encrypt specific JSON fields inside message payloads and store them as encrypted envelopes.
  - **Full-message encryption**: encrypt the entire message bytes (nonce||ciphertext) before storing.
- The same 32‑byte AES key (provided as 64 hex chars) is used for both modes.

Configuration
-------------
ProgressDB reads encryption settings from either `config.yaml` or environment variables.

Environment variables

- `PROGRESSDB_ENCRYPTION_KEY`: 64 hex characters (32 bytes). Example generation:

  ```sh
  openssl rand -hex 32
  ```

- `PROGRESSDB_ENCRYPT_FIELDS`: Optional comma-separated list of JSON paths to encrypt, e.g.

  ```sh
  PROGRESSDB_ENCRYPT_FIELDS=body.credit_card,body.phi.*
  ```

Config file (`config.yaml`) example

```yaml
security:
  encryption_key: "0123..."    # 64 hex chars
  fields:
    - path: "body.credit_card"
      algorithm: "aes-gcm"
    - path: "body.phi.*"
      algorithm: "aes-gcm"
```

How encryption is applied
-------------------------
- On startup the server calls `security.SetKeyHex()` with the configured key. If the key is missing or invalid, encryption is disabled.
- If field rules are configured (`security.SetFieldPolicy()`), then during `SaveMessage` the server attempts to:
  1. Parse the message bytes as JSON and apply `EncryptJSONFields` to encrypt configured paths.
  2. If JSON parsing or field encryption fails, fall back to `Encrypt()` and store the entire message encrypted.
- If no field rules are configured, the server uses `Encrypt()` to encrypt the whole message bytes.
- On reads (`ListMessages`, `ListMessageVersions`, `GetLatestMessage`) the server will try to:
  1. `Decrypt()` as a full-message ciphertext; if that succeeds, return the decrypted bytes.
  2. Otherwise, attempt `DecryptJSONFields()` which discovers envelope objects and decrypts them in-place.
  3. Legacy plaintext JSON is tolerated when appropriate.

Envelope format (field-level encryption)
--------------------------------------
When a JSON field is encrypted the server replaces the value with an "envelope":

```json
{
  "_enc": "gcm",
  "v": "<base64(nonce||ciphertext)>"
}
```

- `_enc` is the algorithm marker (currently `gcm`).
- `v` is base64 encoded `nonce||ciphertext` as produced by AES‑GCM encryption used by the server.

JSON path syntax for field rules
--------------------------------
- Paths are dot-separated segments: `body.credit_card`.
- Wildcard `*` matches all keys in an object or all elements of an array at that level: `body.phi.*`.
- Numeric segment selects an array index: `sections.0.payment`.
- If a path does not exist, it is ignored.

Client-side encryption considerations
-----------------------------------
- Clients can pre-encrypt fields or entire payloads if desired. To be interoperable with the server's decryption, clients should use the same envelope format for field-level encryption or the same nonce||ciphertext layout for full-message encryption.
- If the server does not have the key, it cannot decrypt stored content — this allows for end-to-end encryption if you operate the server without the key, but then server-side features that rely on plaintext will not work.

Key management & rotation
-------------------------
- Store the encryption key outside of source control in a secrets manager (Kubernetes secret, Vault, AWS Secrets Manager, etc.).
- Rotation requires re-encrypting stored data. Common patterns:
  - Re-encrypt on write: when you read a record with the old key, write it back encrypted with the new key.
  - Offline re-encryption job: run a migration that decrypts and re-encrypts records with the new key.
- The server currently supports a single active key; implement a re-encrypt job for rotation.

Operational notes and caveats
----------------------------
- Encrypted fields are opaque: you cannot search or index encrypted content server-side.
- Field-level encryption adds CPU and JSON traversal overhead; benchmark if you process high throughput.
- Use `skipLibCheck` in TypeScript builds (unrelated to encryption) for smoother dev experience, but that does not affect runtime encryption.
- Do not store encryption keys alongside code. Rotate keys periodically and plan a migration strategy.

Examples
--------

1) Selective field encryption via environment:

```sh
export PROGRESSDB_ENCRYPTION_KEY=$(openssl rand -hex 32)
export PROGRESSDB_ENCRYPT_FIELDS="body.credit_card,body.phi.*"
./progressdb --config ./config.yaml
```

2) Message before encryption:

```json
{
  "id": "m1",
  "thread": "t1",
  "author": "u1",
  "body": {
    "text": "hello",
    "credit_card": {"number":"4111...","exp":"12/25"},
    "phi": {"ssn":"123-45-6789"}
  }
}
```

3) After server field-level encryption (stored):

```json
{
  "id": "m1",
  "thread": "t1",
  "author": "u1",
  "body": {
    "text": "hello",
    "credit_card": {"_enc":"gcm","v":"BASE64_NONCE_CIPHERTEXT"},
    "phi": {"_enc":"gcm","v":"BASE64_NONCE_CIPHERTEXT"}
  }
}
```

Support & testing
-----------------
- Unit tests and integration tests should exercise both `EncryptJSONFields` and `DecryptJSONFields`, full-message `Encrypt`/`Decrypt`, and the Save/List code paths with and without field policies configured.

References (code)
-----------------
- `server/pkg/security/crypto.go` — encryption helpers: `SetKeyHex`, `SetFieldPolicy`, `Encrypt`, `Decrypt`, `EncryptJSONFields`, `DecryptJSONFields`.
- `server/pkg/store/pebble.go` — shows how encryption is applied at write/read time.
- `docs/DEPLOY.md` — deployment notes for environment variables and key generation.

