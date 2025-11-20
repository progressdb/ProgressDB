
![ProgressDB Logo](/docs/images/wordlogo.png)

Purpose built database for chat applications, streamlining common workflows and performance requirements:

- **Threaded messages lifecycle** - Built-in versioning, edits, and soft-deletes
- **Optimised mutations & reads** - Fast retrieval of threaded messages and common chat patterns
- **Integrated security** - Encryption and API-key based access controls
- **Developer-friendly** - Lightweight service with simple APIs and SDKs for Python, Node, TypeScript, and React
- **Developer Delight** - Send a message, the thread auto-creates.

ProgressDB eliminates the complexity of building chat infrastructure: fewer transformation layers, direct APIs for threads and messages, and the tooling to move from prototype to production with operational confidence.

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
- [x] Performance - sustains 3k RPS on 2 local CPU cores (3.49 GHz). 

Planned
- [ ] Encryption - cloud-backed KMS, HSM integration, DEK rewrap features
- [ ] Backups - backups & tested restore of chat datas
- [ ] Realtime - realtime subscriptions (WebSocket / SSE) and webhook delivery
- [ ] Search - search API / indexed search experience
- [ ] Scaling - vertical or horizontal scaling features
- [ ] Metrics - Metrics are present, but need cleanup for prod

[![test-db-service](https://github.com/progressdb/ProgressDB/actions/workflows/test-db-service.yml/badge.svg)](https://github.com/progressdb/ProgressDB/actions/workflows/test-db-service.yml)



## Links

- [Documentation](./docs/public/v_0_2_0/)
- [Admin Dashboard](https://progressdb.dev/app/explorer)
- [Explorer Image](/public/dasher.png)