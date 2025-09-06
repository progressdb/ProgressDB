Overview
--------
This guide explains how to start the ProgressDB server locally and how to use the HTTP API for messages and threads (creation, listing, edits/versioning, replies, reactions and soft-deletes).

Quick start (local)
- Ensure you have Go 1.21+ installed.
- From the repo root, build the server:
  - `cd server && go build ./cmd/progressdb`
- Start the server (example):
  - `./cmd/progressdb/progressdb --addr 127.0.0.1:8080 --db ./data`
- Or use the test script to run unit tests and exercise endpoints:
  - `.scripts/run-tests.sh`

Environment & config
- The server reads `config.yaml` and supports `.env` files for overrides. Common env vars:
  - `PROGRESSDB_ADDR` (e.g. `0.0.0.0:8080`)9
  - `PROGRESSDB_DB_PATH` (Pebble DB path)
  - `PROGRESSDB_ENCRYPTION_KEY` (32-byte hex for AES‑GCM)

HTTP API (examples)

- Create a thread:
  - curl -X POST http://localhost:8080/v1/threads -H 'Content-Type: application/json' -d '{"name":"room-1"}'

- Create a message (server fills `id`/`ts` if missing):
  - curl -X POST http://localhost:8080/v1/messages -H 'Content-Type: application/json' -d '{"thread":"thread-1","author":"u1","body":{"text":"hello"}}'

- List messages in a thread:
  - curl 'http://localhost:8080/v1/messages?thread=thread-1&limit=50'

- Get latest message by id:
  - curl http://localhost:8080/v1/messages/msg-1700000000-1

- Edit a message (append a new version):
  - curl -X PUT http://localhost:8080/v1/messages/{id} -H 'Content-Type: application/json' -d '{"id":"<id>","thread":"<thread>","author":"u1","body":{"text":"edited"}}'

- List versions for a message id:
  - curl http://localhost:8080/v1/messages/{id}/versions

- Soft-delete a message:
  - curl -X DELETE http://localhost:8080/v1/messages/{id}

- Reply to a message:
  - curl -X POST http://localhost:8080/v1/messages -H 'Content-Type: application/json' -d '{"thread":"thread-1","author":"u2","reply_to":"<msg-id>","body":{"text":"reply text"}}'

- Add a reaction (simple approach): update the message with a new `reactions` map via PUT
 - Add a reaction (recommended): call the reactions API. Identity ids are opaque
   to the server and may represent users, groups, or anything the client understands.
   - Add/Update reaction (body: {"id":"<identity>","reaction":"<string>"}):
     - curl -X POST http://localhost:8080/v1/messages/<msg-id>/reactions -H 'Content-Type: application/json' -d '{"id":"id-1","reaction":"👍"}'
   - List reactions:
     - curl http://localhost:8080/v1/messages/<msg-id>/reactions
- Remove reaction for an identity:
  - curl -X DELETE http://localhost:8080/v1/messages/<msg-id>/reactions/id-1

Admin examples
- Check admin health:
  - curl -H "Authorization: Bearer <ADMIN_KEY>" http://localhost:8080/admin/health
- Get admin stats:
  - curl -H "Authorization: Bearer <ADMIN_KEY>" http://localhost:8080/admin/stats
- List threads (admin):
  - curl -H "Authorization: Bearer <ADMIN_KEY>" http://localhost:8080/admin/threads

OpenAPI & docs
- The OpenAPI spec is in `server/docs/openapi.yaml` and the server exposes it for Swagger UI. Use the spec to generate clients.

Notes
- Messages are append-only; edits and deletes append new versions. Versions are indexed by `msgid:<id>:<ts>-<seq>` keys so you can list historical versions.
- Threads are lightweight metadata stored at `threadmeta:<id>`.
- For production usage, configure TLS, API keys, and consider key rotation for encryption.
