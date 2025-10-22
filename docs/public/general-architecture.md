---
section: general
title: "Architecture"
order: 4
visibility: public
---

# System Architecture Overview

ProgressDB is built as a **high-performance, append-only database service**, purpose-designed for chat and timeline data. Its architecture emphasizes predictable performance, data integrity, and developer-friendly APIs.

---

## 1. Interface Layer

- **Transport:** All client and system integrations use **HTTP(S)** as the protocol, with a simple REST-like API surface.
- **Encoding:** Payloads are encoded in **JSON** for universal compatibility and ease of use across languages and platforms.

### Request Lifecycle

1. **Client Request**: A client (web, mobile, server, or bot) sends a JSON-encoded request (e.g., create message, fetch thread) via HTTP POST or GET.
2. **Authentication**: The API performs authentication using **API keys** or signed requests.
3. **Ingress Queuing**: Validated requests are placed into a high-throughput, in-memory **ingress queue**.

---

## 2. Ingestion Pipeline

The core logic is organized as a **pipeline of distinct, composable stages**:

1. **Intake Queue**: Buffers incoming operations for burst handling and fair scheduling.
   - Highly concurrent design — engineered for chat workloads with massives spikes in activity.
2. **Validation & Preprocessing**: Checks payloads for schema correctness, required fields, and security policies.
3. **Batching**: Incoming events are grouped (batched) both per logical scope (e.g., thread, conversation) and for optimal write throughput.
   - Configurable batch size and timeout for fine-grained performance/durability tuning.
4. **Application Layer**: Applies batched operations to the underlying storage engine, appending new events.
5. **Durability/Sync**: Data is committed. If **WAL (Write-Ahead Logging)** is enabled, batch state is mirrored for full durability and crash recovery.
6. **Ack/Respond**: Once fully applied, the service acknowledges completion with a JSON response.

---

## 3. Storage Engine

- **Append-Only Log:** All data is modeled as a single, append-only log per logical scope (thread, conversation, etc).
- **Immutable Events:** Updates, deletes, and edits become new log entries rather than in-place mutations.
- **Batch Writes:** Consistency and durability are guaranteed through atomic batch operations.

---

## 4. Query & Read Paths

- **Thread/Message Retrieval:** Reads return a **point-in-time consistent view** based on the log’s state. Recent events are cached for low-latency scroll/tail scenarios.
- **Denormalization:** Internal structures are denormalized for optimized read speed — most read queries avoid joins or index lookups.
- **Live Tailing:** Clients can "tail" a thread to receive new messages with minimal lag, ideal for chat and real-time workloads.

---

## 5. Additional System Features

- **API Key System:** Supports fine-grained permissioning for clients, frontends, backends, and admins.
- **CORS & Origin Controls:** Tunable for secure deployment in public or private network scenarios.
- **Telemetry & Monitoring:** Built-in sampling and slow-query telemetry; easily integrates with external systems.
- **Resource Guards:** Rate limits, IP allowlists, and retention policies can be enforced as needed.

---

## Summary Diagram

```
Client (HTTP/JSON)
       |
   [API Frontend]
       |
 Ingress Queue  <--- Telemetry, Auth, Rate-limiting
       |
  Validation & Preprocessing
       |
   Batching & Apply (Atomic)
       |
  [Storage Engine: Append-only log]
       |
   Read APIs / Live Tail
```

---

This architectural pipeline is optimized for real-world chat workloads, ensuring **predictable latency, durability, and auditability**—while keeping the operational model and APIs minimal and approachable.