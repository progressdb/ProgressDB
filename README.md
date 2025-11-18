
![ProgressDB Logo](/docs/images/wordlogo.png)

ProgressDB is a fast, purpose-built, chat-native database for AI chat threads. The project includes a database service, backend SDKs (Node, Python), and frontend SDKs (TypeScript, React). This quickstart shows how to run the service locally, install the SDKs, and perform basic operations.

>ProgressDB is in active development and not certified for production use.
>While extensively tested, breaking changes and incomplete features remain.
>The built-in Progressor handles automatic database migrations on model changes, though this currently applies only to the database layer—not the SDKs.

## Why ProgressDB?

ProgressDB is built specifically for chat threads and makes common chat workflows & workloads simple and fast:

- Start a thread by sending a message — the database will auto-create the thread for you.
- Built-in message versioning, edits, replies, reactions, and soft-deletes.
- Optimized for fast threaded message retrievals and common chat patterns.
- Straightforward encryption and API-key based access controls.
- Ship quickly: small service, simple APIs, and SDKs for Python, Node and frontend use.

ProgressDB removes friction when building chat-first apps or features: fewer transformation layers, direct APIs for threads/messages, and tooling to get you from prototype to production faster with operational clarity.

#### Without ProgressDB, storing chat data becomes:

- Week 2: “Need message versions.”
- Week 4: “Need soft deletes + GDPR.”
- Week 6: “Need encryption.”
- Week 8: “Search is slow → add Elasticsearch.”
- Week 10: “Need real-time.”
- Week 12: “Encryption model is wrong.”
- Week 16: “Reads slow → add Redis.”
- Week 20: “Need encryption key rotation.”


[![Docker Pulls](https://img.shields.io/docker/pulls/progressdb/progressdb?logo=docker)](https://hub.docker.com/r/progressdb/progressdb)

## Features

Available
- [x] Messages - append-only storage, versioning (edits), replies, soft-delete
- [x] Threads - metadata operations (create/update/list)
- [x] Encryption & Key Management - (embedded KMS mode)
- [x] Retention - policy-driven purge/run hooks
- [x] Backend SDKs - node & python sdks published for ^v0.2.0
- [x] Frontend SDKs - typescript & react sdks published for ^v0.2.0
- [x] Reliability - (appWAL/buffering) are present & configurable
- [x] Performance - preliminary tests estimate 2vcpu nets 3k RPS. 

Planned
- [ ] Encryption - cloud-backed KMS ~ HSM integration from local only / dek rewrap feature
- [ ] Backups - backups & tested restore of chat datas
- [ ] Realtime - realtime subscriptions (WebSocket / SSE) and webhook delivery
- [ ] Search - search API / indexed search experience
- [ ] Scaling - vertical or horizontal scaling features
- [ ] Metrics - Metrics are present, but need cleanup for prod

[![test-db-service](https://github.com/progressdb/ProgressDB/actions/workflows/test-db-service.yml/badge.svg)](https://github.com/progressdb/ProgressDB/actions/workflows/test-db-service.yml)
