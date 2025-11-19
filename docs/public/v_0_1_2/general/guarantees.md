---
section: general
title: "Guarantees"
order: 3
visibility: public
---

# Traditional Guarantees — Writes

## Atomic & Durable Writes
All mutative operations either **succeed or fail together** by using grouped batch writes.
Each batch write is made **durable immediately** after commit.

## Consistent Writes
All write operations maintain a **global order of execution** based on the request’s received time.
This ensures that batches are applied in strict sequence.

### Consistency Within Scoped Operations
- All mutations within a logical scope (e.g., `threads`, `messages`, etc.) are **batched and written atomically**.
- The batch is applied in sequence order, preserving full write consistency within that scope.

## Durable Writes
All committed batch writes are **durable upon completion**.
If **application-level WAL** is enabled, durability is maintained through the entire write pipeline, providing **automatic crash recovery**.

## Isolated Writes
Each batch apply cycle operates **only on its current scoped group** (e.g., thread or message operations).
This ensures **write isolation** — operations from other scopes cannot interfere during the same batch cycle.

---

# Traditional Guarantees — Reads

## Consistent Reads
All reads (user-scoped or thread-scoped) return a **point-in-time consistent view** of the data, ensured by the atomicity of underlying batch writes.
Reads never see partial or in-progress writes.

---

# Performance Guarantees

## Ultra-Fast Processing — ~1 ms Loss Window (Configurable)
All mutative requests (`create`, `update`, `delete`) are acknowledged and processed **within sub-millisecond latency** under normal load.

Because of the design’s write-optimized path, most requests complete **within a 1 ms window**.

This database is optimized **specifically for chat workloads** — the access and mutation patterns are baked into the design for predictable performance.


# Maintaining Fast Processing

In chat workloads, predictable latency is often more valuable than absolute durability — users prioritize instant responsiveness over the rare loss of a transient write in the event of a crash.

When the intake WAL is disabled, the system operates with a typical loss window of ~1 ms, enabling sub-millisecond write acknowledgments.

However, when the intake WAL is enabled, each write incurs additional latency due to a full fsync configuration (forced disk write) before acknowledgment, ensuring stronger durability guarantees.

Ongoing research and optimization efforts aim to further refine this balance between performance, durability, and latency with softer/batched durability configurations in next releases.

---

# Complete Durability Mode (WAL)
When **Durable Mode** is enabled:

- Every acknowledged write is guaranteed to be recoverable, even after process or system crashes.
- No buffering or time-window batching is applied — **each write is fully fsynced**.

Latency and throughput costs depend on your compute and storage capabilities.
On average, each `fsync` adds **~2 ms per operation**, trading performance for absolute durability.

---

# Summary: Tunable Tradeoffs and chat Optimized Guarantees

ProgressDB delivers atomicity, consistency, and isolation by design, adapting durability to match your specific requirements via configuration.

By optimizing isolation at logical scope levels (such as threads or conversations), ProgressDB achieves high throughput and predictable consistency for chat-centric workloads, rather than aiming for traditional SQL-style broad transactional semantics.

This lets you tailor the database's performance and durability balance for your chat application, with full clarity on what aspects are tunable versus guaranteed.
