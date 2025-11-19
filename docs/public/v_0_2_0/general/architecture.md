---
section: general
title: "Architecture"
order: 4
visibility: public
---

# Architecture

The database service is tuned for for real-time chat writes and reads.  
Principle is **consistently fast latency, chat optimized read, write access patterns, and operational simplicity**.

## Core Principles

- **Predictable Latency**: Sub-millisecond write acknowledgment with configurable durability.
- **Chat-Optimized Storage**: Threads and their messages are stored contigiously on disk, avoiding the sparse, fragmented storage patterns typical in SQL and NoSQL systems.
- **Simple Interface**: Clean REST API, JSON payloads, familiar web development patterns.

## System Layers

- **Interface**: HTTP(S) REST API, authentication, request validation.
- **Ingestion**: Buffered intake, batch processing, atomic writes, configurable durability.
- **Storage**: Versioned updates, change tracking, thread-level isolation.
- **Query**: Point-in-time consistent reads, indexed, denormalized for chat patterns, real-time tailing.
- **Integration**: Flexible formats, message queue and object store connectivity, single-binary deployment.

## Performance & Operational Highlights

- Sub-millisecond writes, Sub-millisecond reads or sub 5ms tailed reads.  
- Optimized for chat workloads, all normal database baggage is no more.
- Full resource utilization, threaded multi processing etc.
- Production-ready: telemetry, monitoring, graceful degradation