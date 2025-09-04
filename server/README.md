ProgressDB Server

Overview
- HTTP server that appends and lists messages per thread.
- PebbleDB as the storage engine.
- Optional AES‑256‑GCM encryption for stored messages.

Run
- Start: `./scripts/start.sh --addr :8080 --db ./data`
- Build: `./scripts/build.sh`

API
- GET `/?thread=<id>&msg=<text>`: Append message and list.
- GET `/?thread=<id>`: List messages.
- POST `/` (form: `thread`, `msg`): Append and list.

Data Model
- Message: fixed metadata + flexible payload.
  - id: unique message id (string, e.g., `msg-123`).
  - thread: thread id (string, e.g., `thread-9`).
  - author: author/user id (string, e.g., `user-5`).
  - ts: unix timestamp seconds (int) or nanos (int64).
  - body: freeform JSON payload defined by the customer.
  - example:
    {"id":"msg-123","thread":"thread-9","author":"user-5","ts":1693888302,"body":{"text":"hello","tags":["greeting"]}}
- Thread: minimal metadata with rollups.
  - id: thread id (string).
  - created_ts: first message timestamp.
  - last_ts: last message timestamp.
  - message_count: count of messages (optional to maintain in MVP).
  - attributes: optional freeform JSON (e.g., title/subject).

Keyspace & Indexing
- Primary storage (append-ordered by time):
  - key: `thread:<threadID>:<unix_nano>-<seq>`
  - val: message bytes (MVP: plaintext or encrypted; future: JSON-encoded message object).
- Listing a thread: iterate by `prefix = "thread:" + threadID + ":"`.
- Secondary indexes (future):
  - by message id: `msg:<id> -> pointer or full JSON`.
  - by author: `author:<authorID>:<unix_nano>-<seq> -> msg id`.

Schema Strategy
- MVP: fixed metadata + flexible `body` JSON for customer-defined payloads.
- Optional schema registration (future): allow customers to supply JSON Schema/Protobuf for validation and richer queries.

Configuration
- Config file: `config.yaml` (YAML)
  - server.address: host/interface (e.g., `0.0.0.0`)
  - server.port: 8080
  - storage.db_path: `./data/progressdb`
  - security.encryption_key: 32‑byte hex (AES‑256‑GCM)
  - logging.level: `info`

Environment Variables
- `.env` support: if a `.env` file is present in the working directory, it is loaded at startup.
- Variables:
  - `PROGRESSDB_ADDR`: full listen addr `host:port` (e.g., `0.0.0.0:8080`).
  - `PROGRESSDB_ADDRESS`: host/interface (used with `PROGRESSDB_PORT`).
  - `PROGRESSDB_PORT`: port number (string accepted, e.g., `8080`).
  - `PROGRESSDB_DB_PATH`: Pebble database path.
  - `PROGRESSDB_ENCRYPTION_KEY`: 64‑hex chars (32 bytes) for AES‑256‑GCM.
  - `PROGRESSDB_CONFIG`: path to `config.yaml` (optional; you can also pass `--config`).

Precedence
- Flags explicitly set > Environment variables > Config file values > Built‑in defaults.

Security Notes
- Keep `config.yaml` and `.env` out of version control.
- Set permissions to owner‑read/write only (e.g., `chmod 600 config.yaml .env`).
- Rotating the encryption key on existing data is non‑trivial; plan a migration.

Encryption Policy
- Full-message encryption (default when key is set):
  - Writes: value bytes are encrypted with AES‑256‑GCM (nonce|ciphertext stored).
  - Reads: values are decrypted transparently before returning via API.
- Selective field-level encryption (configurable):
  - Define JSON paths under `security.fields` or via `PROGRESSDB_ENCRYPT_FIELDS` (comma-separated paths).
  - Example config:
    encryption:
      fields:
        - path: body.credit_card
          algorithm: aes-gcm
        - path: body.phi.*
          algorithm: aes-gcm
  - Write path: encrypt configured fields before persist; non-JSON payloads fall back to full-message encryption.
  - Read path: decrypt envelopes on matched fields; full-message decryption is attempted first for backwards compatibility.

Field-Level Encryption
- Goal: allow encrypting specific JSON fields while keeping indexable metadata clear (thread/id/author/ts remain plaintext).
- Path syntax (proposed):
  - Dot notation relative to the message root, e.g., `body.credit_card`, `body.phi.*`.
  - `*` matches a single object key level (no deep recursion).
  - Array indices supported with numeric selectors, e.g., `body.items[0].secret` (wildcards for arrays may be added later).
  - Only `body.*` paths are recommended; encrypting top-level metadata would break indexing.
- Storage format:
  - Replace the plaintext field value with an envelope object:
    {"_enc":"gcm","v":"<base64(nonce|ciphertext)>"}
  - Keeps the JSON shape intact and signals encrypted fields explicitly.
- Write pipeline:
  - Parse message JSON, traverse paths, and for each match:
    - Serialize field value to JSON bytes, encrypt with AES‑GCM (single instance key), base64 it.
    - Replace the field value with the encryption envelope.
  - Persist the resulting message JSON as the record value (existing nonce|ciphertext-at-value option remains for full-message mode).
- Read pipeline:
  - Parse fetched message JSON and traverse; where envelope objects are found, decrypt back to the original JSON value.
  - If decryption fails and policy requires, return an error; otherwise, redact or pass envelope through (configurable).
- Backwards compatibility:
  - Old records without envelopes are treated as plaintext.
  - Full-message encrypted records continue to be supported; field-level and full-message modes are mutually exclusive per deployment.
- Operational notes:
  - Key: single symmetric key via `PROGRESSDB_ENCRYPTION_KEY` or config.
  - Rotation: plan a re-encrypt migration; envelopes allow in-place rotation with versioning (e.g., add `_kid`).
  - Performance: path matching and per-field crypto add overhead; cache compiled path matchers.

Examples
- Minimal `.env`:
  - `PROGRESSDB_ADDR=0.0.0.0:8080`
  - `PROGRESSDB_DB_PATH=./data/progressdb`
  - `PROGRESSDB_ENCRYPTION_KEY=b36ef5f7c11c1d29ab0b22789d9ed4b99f6b84c6a2a8f7f93c8f33485bc23a12`

- Minimal `config.yaml`:
  server:
    address: "0.0.0.0"
    port: 8080
  storage:
    db_path: "./data/progressdb"
  security:
    encryption_key: "b36ef5f7c11c1d29ab0b22789d9ed4b99f6b84c6a2a8f7f93c8f33485bc23a12"
  logging:
    level: "info"
