---
section: general
title: "Concepts"
order: 2
visibility: public
---

# Concepts

This page explains ProgressDB's core concepts so you can design your
application and operate the service effectively.

Threads

- A thread is a logical conversation or channel that contains messages.
- Thread metadata includes `id`, `created_ts`, `last_ts`, and optional `attributes` (freeform JSON).
- Threads are created implicitly when the first message in a thread is posted or explicitly via `POST /v1/threads`.

Messages

- Messages are the primary append-only items stored in a thread. Each message has:
  - `id` — message id (server can generate when omitted).
  - `thread` — thread id.
  - `author` — author/user id (string).
  - `ts` — timestamp (seconds or nanos; server will fill when omitted).
  - `body` — freeform JSON payload owned by the application.
- The server supports message edits and versions. Versions are stored and can be listed via `GET /v1/threads/{threadID}/messages/{id}/versions`.
- Soft-deletes are supported: deletes mark a message as deleted while preserving history.

Indexing & storage model

- Primary storage uses a Pebble DB with a time-ordered keyspace to make timeline reads efficient.
- Thread listing is implemented by iterating keys with the prefix `thread:<threadID>:`.
- Secondary indexes (message id lookup, author lookup) exist as compact keys that map to pointers or full JSON values.

Authentication & signing

- API keys are required for all requests. Use `Authorization: Bearer <key>` or `X-API-Key: <key>`.
- Backend keys (`sk_...`) have elevated privileges and may call `POST /v1/_sign` to generate an HMAC-SHA256 signature for a `userId`.
- Frontend flows: backends sign a user id and return `{ userId, signature }` to the client. Clients include `X-User-ID` and `X-User-Signature` headers on requests to assert identity.

Encryption & KMS

- ProgressDB supports field-level encryption: configured JSON paths are encrypted on write and decrypted on read.
- The server delegates encryption key management to a KMS. Two modes exist:
  - `embedded`: in-process KMS (suitable for dev/test only).
  - `external`: recommended for production; the server calls an external `progressdb-kms` over HTTP/TCP.
- Wrapped DEKs, metadata, and KMS audit logs are persisted under the KMS data directory and are used during rotation/rewrap.

Operational considerations

- Back up the Pebble DB directory (`--db`) before upgrades or rewrap operations.
- Monitor `/metrics` with Prometheus and use `/healthz` for readiness checks.
- Protect API keys and the KMS master key (or KMS service) with a secrets manager and network controls.

