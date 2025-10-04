---
section: general
title: "Welcome"
order: 1
visibility: public
---

<!-- ProgressDB Logo -->

# Welcome to ProgressDB

ProgressDB is a purpose-built, chat-native database for storing and
retrieving chat threads and messages with first-class support for versioning,
edits, replies, reactions, and optional encryption. The project includes a
lightweight HTTP server, backend SDKs (Node, Python), and frontend SDKs
(TypeScript/React).

Key capabilities

- Thread-centric storage optimized for append and timeline reads.
- Message versioning and soft-deletes so edits are traceable.
- Optional field-level encryption backed by a KMS for sensitive data.
- Simple API (REST + OpenAPI) and small footprint suitable for prototypes and production.

Quickstart (local)

1. Run the server (Docker or `go run`):

```sh
# Docker example
docker run -p 8080:8080 -v $PWD/data:/data docker.io/progressdb/progressdb --db /data/progressdb

# or from source
cd server
go run ./cmd/progressdb --db ./data --addr ":8080"
```

2. Explore the API docs: `http://localhost:8080/docs/` and admin viewer at `http://localhost:8080/viewer/`.

3. Use an SDK or curl to create threads and messages. Example message POST:

```sh
curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer pk_example" \
  -H "Content-Type: application/json" \
  -d '{"thread":"general","author":"alice","body":{"text":"Hello"}}'
```

Where to go next

- Installation and flags: `docs/public/guides-installation.md`.
- Configuration and KMS: `docs/public/service-configuration.md` and `server/docs/kms.md`.
- API design & concepts: `docs/public/concepts.md`.

