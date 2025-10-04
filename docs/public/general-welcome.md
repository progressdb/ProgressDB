---
section: general
title: "Welcome"
order: 1
visibility: public
---

<!-- ProgressDB Logo -->

# Welcome to ProgressDB

ProgressDB is a chat-native database engineered for storing and
serving conversational data (threads and messages) with high efficiency
and predictable behavior. Unlike general-purpose document or relational
databases, ProgressDB focuses on the common patterns of chat systems:
append-only timelines, fast timeline reads, message versioning, and
field-level encryption for sensitive data.

## Why ProgressDB — a short rationale

- Purpose-built indexing: keys are organized for append-ordered timeline
  access (e.g. `thread:<id>:<timestamp>`). This makes listing a thread's
  messages extremely efficient compared to full table scans or wide
  secondary queries.
- Small, predictable storage engine: uses PebbleDB as an embedded key/value
  store which provides fast local IO, compaction, and low operational
  overhead. No heavyweight cluster coordination is required for single-node
  deployments.
- First-class chat features: message versioning, edits, replies, reactions
  and soft-deletes are built-in primitives — you get auditability without
  complex application-level bookkeeping.
- Security-first: optional field-level encryption backed by a KMS and a
  signing flow for safe frontend usage (clients never hold backend keys).

## How ProgressDB is faster for chat workloads

- Timeline-optimized keys: by storing timeline data in append-ordered keys
  and scanning by prefix, the server avoids expensive secondary index
  maintenance and returns timeline pages with a single sequential read.
- Compact object model: messages are JSON with small fixed metadata; the
  server avoids relational joins and large deserialization costs on reads.
- Lightweight runtime: a small Go binary with tuned concurrency and
  Pebble optimizations means lower CPU/memory overhead and quicker cold
  starts compared to distributed solutions.

## Typical use-cases

- Chat applications (customer support, group chat, bot conversations)
- AI conversation stores (chat history for LLM contexts)
- Audit logs where message versioning and soft-deletes are required
- Prototypes and single-region services where simple operational model is
  preferred over distributed databases

## Trade-offs & when not to use ProgressDB

- Not a drop-in replacement for massive multi-region OLTP systems that
  need cross-shard transactions or advanced analytical queries.
- If you require global multi-master replication or sub-second cross-region
  consistency, consider a distributed DB built for that purpose and use
  ProgressDB as a local cache or shard for chat workloads.

## Quickstart (local)

Run the server via Docker:

```sh
docker run -p 8080:8080 -v $PWD/data:/data docker.io/progressdb/progressdb --db /data/progressdb
```

Or run from source (developer):

```sh
cd server
go run ./cmd/progressdb --db ./data --addr ":8080"
```

Explore the API docs: `http://localhost:8080/docs/` and admin viewer at
`http://localhost:8080/viewer/`.

Create a message (curl):

```sh
curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer pk_example" \
  -H "Content-Type: application/json" \
  -d '{"thread":"general","author":"alice","body":{"text":"Hello"}}'
```