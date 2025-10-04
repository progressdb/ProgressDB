---
section: general
title: "Concepts"
order: 2
visibility: public
---

# Concepts

This page explains ProgressDB's core concepts so you can design your
application and operate the service effectively.

## Threads

- A thread is a logical conversation or channel that contains messages.
- Thread metadata includes `id`, `created_ts`, `last_ts`, and optional `attributes` (freeform JSON).
- Threads are created implicitly when the first message in a thread is posted or explicitly via `POST /v1/threads`.

### Thread fields

- `id` — unique thread identifier (string).
- `created_ts` — timestamp of first message.
- `last_ts` — timestamp of most recent message.
- `attributes` — optional freeform JSON (title, description, custom metadata).

## Messages

Messages are the primary append-only items stored in a thread.

### Message fields

- `id` — message id (server generates when omitted).
- `thread` — associated thread id.
- `author` — author/user id.
- `ts` — timestamp (seconds or nanos); server fills when omitted.
- `body` — freeform JSON payload owned by the application.

### Message lifecycle

- Edits: messages support versioning; each edit creates a new stored version.
- Versions: listable via `GET /v1/threads/{threadID}/messages/{id}/versions`.
- Soft-deletes: messages can be deleted while preserving history for audit.

## Indexing & storage model

- Storage engine: Pebble DB with time-ordered keys for efficient timeline reads.
- Primary key layout: `thread:<threadID>:<unix_nano>-<seq>` (append-ordered timeline keys).
- Thread listing: iterate keys with prefix `thread:<threadID>:`.
- Secondary indexes: compact keys for message id lookup and author-based indexes.

## Authentication & signing

- API key authentication: provide keys via `Authorization: Bearer <key>` or `X-API-Key: <key>`.
- Key scopes:
  - Backend keys (`sk_...`) — full privileges, may call `POST /v1/_sign` and admin routes.
  - Frontend keys (`pk_...`) — limited scope (message endpoints, health).

### Signing flow (frontend)

1. Backend calls `POST /v1/_sign` with `{ "userId": "..." }` using a backend key.
2. Server returns `{ "userId": "...", "signature": "..." }` (HMAC‑SHA256).
3. Client includes `X-User-ID` and `X-User-Signature` headers on requests to assert identity.

## Encryption & KMS

- ProgressDB supports optional field-level encryption (encrypt specific JSON paths) and full-message encryption modes.
- KMS modes:
  - `embedded`: in-process KMS (development/testing).
  - `external`: production recommended; calls an external `progressdb-kms` daemon over HTTP/TCP.
- Wrapped DEKs, metadata, and KMS audit logs are persisted under the KMS data directory and are used for rotation and rewrap operations.

### Field-level encryption

- Configure JSON paths to encrypt (e.g., `body.credit_card`). The server replaces encrypted fields with an envelope object like `{ "_enc": "gcm", "v": "<base64>" }`.
- On reads the server decrypts envelopes and returns original JSON values (when the KMS is available and keys are valid).

## Operational considerations

- Backups: snapshot the Pebble DB path (`--db`) and KMS data directory before upgrades or rewraps.
- Monitoring: scrape `/metrics` with Prometheus and use `/healthz` for readiness probes.
- Security: protect API keys and KMS access using a secrets manager and network controls.

## Examples

Create a message (curl):

```sh
curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer pk_example" \
  -H "Content-Type: application/json" \
  -d '{"thread":"general","author":"alice","body":{"text":"Hello"}}'
```

List message versions (curl):

```sh
curl http://localhost:8080/v1/threads/general/messages/msg-123/versions \
  -H "Authorization: Bearer sk_example"
```
