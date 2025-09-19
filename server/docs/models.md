Data models & storage
=====================

Overview
--------

This document describes the core data models used by ProgressDB (messages and threads), how they are stored in Pebble, and the key formats and relationships used to retrieve and index data efficiently.

Models
------

Message
- **id**: string — opaque message id (clients may provide one, server will fill if missing).
- **thread**: string — thread id the message belongs to.
- **author**: string — author identity (the server enforces this from the signing middleware when present).
- **ts**: int64 — UnixNano timestamp assigned by the server when saving a version.
- **body**: object|string — the message payload (usually JSON with `text` and other application fields).
- **reactions**: map[string]string — optional map of identity -> reaction string.
- **deleted**: bool — soft-delete flag stored on later versions.

Thread
- **id**: string — thread identifier.
- **title** / **name**: string — optional human-friendly metadata.
- **author**: string — creator id (optional).
- **createdTS**, **updatedTS**: int64 — timestamps.

Storage design (Pebble keys)
---------------------------

We use a small set of deterministic key prefixes so operations are efficient with prefix scans.

- Message timeline (insertion-ordered):
  - Key format: ``thread:{threadID}:msg:{unix_nano_padded}-{seq}``
    - Example: ``thread:thread-1757161423575196000:msg:01757161423575196000-000001``
  - Value: message JSON (optionally encrypted). These keys are created for each saved message/version in chronological order.

- Message versions index (lookup by id):
  - Key format: ``version:msg:{msgID}:{unix_nano_padded}-{seq}``
    - Example: ``version:msg:msg-1700000000-1:01700000000000000000-000001``
  - Purpose: quick listing of all versions for a logical message id; iteration by this prefix returns chronological versions for the id.

- Thread metadata (fast thread listing & updates):
  - Key format: ``thread:{threadID}:meta``
  - Value: JSON blob containing thread fields (id, title, createdTS, updatedTS, author, etc.).
  - Rationale: a dedicated, compact key for thread metadata makes ``ListThreads()`` cheap and atomic.

Key relationships and common operations
-------------------------------------

- List messages in a thread:
  - Scan keys with prefix ``thread:{threadID}:msg:`` and return values in key order (most recent at end).

- Get all versions of a message id:
  - Scan keys with prefix ``version:msg:{msgID}:``; each value is one version.

- Get latest version for an id:
  - Call `ListMessageVersions(msgID)` and take the last element. There is also a fallback scan over `thread:{id}:meta` entries if older data uses a different index (legacy data).

- List threads:
  - Scan keys with prefix ``thread:`` and read the `:meta` entries (``thread:{id}:meta``) to decode the small JSON values.

Edits, deletes, reactions
-------------------------

-- Edits create new versions: PUT or edit operations do not overwrite existing versions; they append a new version key under ``version:msg:{msgID}:`` with a later timestamp.
- Deletes are soft — a delete appends a new version with `deleted: true`.
- Reactions are stored as part of the message value (a `reactions` map). The recommended client API uses a dedicated reactions endpoint that modifies the latest message version by appending a new version with the updated `reactions` map.

Encryption & field policies
---------------------------

- If encryption is enabled in config, values written to Pebble may be fully encrypted or have selected JSON fields encrypted depending on the field policy. The store code attempts field-level decryption first when reading, then falls back to full-message decryption and finally tolerates plaintext JSON for legacy data.

Runtime & security keys (non-storage)
------------------------------------

- Backend, frontend and admin API keys are managed in the server config and used by the security middleware to determine role (``backend``, ``frontend``, ``admin``).
- Signing keys (HMAC) are used to verify author identity headers — the middleware injects the verified author id into the request context which handlers use as the authoritative author.

Admin access and data viewer
----------------------------

- The server exposes admin endpoints to list keys and fetch a single key value (for an in-application data viewer). These call into the same Pebble store functions described above (`ListKeys`, `GetKey`). When viewing values, be mindful that encrypted content will be returned as stored (possibly encrypted).

Examples
--------

- Message JSON (stored value):

```
{
  "id": "msg-1700000000-1",
  "thread": "thread-1",
  "author": "alice",
  "ts": 1700000000000000000,
  "body": {"text": "Hello world"},
  "reactions": {"u1":"👍"},
  "deleted": false
}
```

-- Thread metadata JSON (value at ``thread:{id}:meta``):

```
{
  "id": "thread-1",
  "title": "Room 1",
  "author": "alice",
  "createdTS": 1700000000000000000,
  "updatedTS": 1700000000000000000
}
```

Notes and recommendations
-------------------------

-- Keep thread metadata separate (``thread:{id}:meta``) for fast thread listing and cheap updates.
-- The ``version:msg:`` index is essential for efficient version lookups; do not remove it unless replacing with an alternative index.
- Avoid storing large binary blobs inline in message values — if needed, store references and keep message JSON small.
- Logs and admin endpoints can help inspect raw keys — use them with caution that encrypted values may not be human-readable.

NOTE: Thread metadata is stored at `thread:<id>:meta`. If automatic JSON decryption is desired in the viewer, that requires access to runtime encryption keys and is not enabled by default.

Deep dive: duplicates, prefixes and why they matter
--------------------------------------------------

This section expands on the intentional duplication of message values across multiple keys, explains each prefix and combination in detail, and describes the performance trade-offs that led to this design.

What the duplicates are
Each saved message/version is written to two places:
  1. Timeline (append): `thread:{threadID}:msg:{ts}-{seq}` — the thread-ordered timeline.
  2. Message-version index: `version:msg:{msgID}:{ts}-{seq}` — the per-message versions index.

These are duplicates of the same JSON value (possibly encrypted). The duplication is a deliberate denormalization to make different read patterns efficient.

Key prefixes and their roles
- `thread:<threadID>:...` (timeline keys)
  - Purpose: efficient listing of messages for a single thread in insertion order.
  - Use cases: show last N messages, paginate thread history, stream new messages.

`version:msg:{msgID}:...` (versions index)
  - Purpose: collect all stored versions of a single logical message ID.
  - Use cases: show edit history for a message, fetch latest by id, audit/versioning.

`thread:{threadID}:meta` (thread metadata)
  - Purpose: small summary record for thread headers (title, author, timestamps).
  - Use cases: fast thread listing, filters, updating thread title/updatedTS.

- Other: `thread:<id>:meta` is an optional alternative namespace if you prefer grouping metadata under the thread prefix (purely organizational).

How combinations translate to queries
Timeline view (fast): iterate `thread:{threadID}:msg:` — touches only keys for that thread.
Message history (fast): iterate `version:msg:{msgID}:` — touches only versions for that id.
Latest by id: take last entry of `version:msg:{msgID}:` (cheap).
Thread list: iterate `thread:` and read `:meta` entries (cheap — metadata is small).

Performance benefit (concrete example)
- Scenario: DB contains 1,000,000 total message records across many threads. A single active thread has 100 messages.
- Timeline query cost with current design:
  - Scan prefix `thread:<that-thread>:` — read ~100 values. Low latency.
- Timeline query cost without timeline keys (single namespace only):
  - You would need to scan all message keys (1,000,000) to find those belonging to the thread or maintain a complex index. This is ~10,000x more keys read in this example.
- Disk and CPU: fewer keys read => fewer decompression/decryption attempts and lower CPU use. Denormalization trades a small storage/write cost for large read-time savings.

Implications and trade-offs
- Storage overhead: each message write stores two values — increases disk usage and write IOPS modestly.
- Write cost: two writes per logical message/version instead of one. Acceptable when reads are much more frequent or latency-sensitive.
- Consistency model: writes are independent; there is a small window where one key may be visible and the other not (eventual consistency). Handlers tolerate transient mismatches.
- Complexity: code must be careful not to double-return the same logical message if it merges results from timeline and version indexes; clients should dedupe by message id when combining sources.

Practical recommendations
- Keep both namespaces and teach clients/handlers to use the right query for the right job:
  - Use timeline keys for thread views.
  - Use msgid index for version/history views.
  - Use `thread:<id>:meta` for thread lists and header displays.
- When rendering a combined view, deduplicate by `id` and prefer the latest `ts`.
  - If you prefer a different organization, the minimal, low-risk change is to use `thread:<id>:meta` so metadata groups under the thread prefix without changing behavior.
