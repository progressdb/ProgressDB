---
section: general
title: "Welcome"
order: 1
visibility: public
---

<!-- ProgressDB Logo -->

# Welcome to ProgressDB

ProgressDB is a chat-native database engineered for storing and serving conversational data — threads, messages, and related context — with high efficiency and predictable behavior.

Unlike general-purpose document or relational databases, ProgressDB is optimized for the patterns of chat systems: append-only timelines, fast ordered reads, message versioning, and field-level encryption for sensitive content.

## Why ProgressDB

### Purpose-built indexing

Keys are organized for append-ordered timeline access (e.g. `thread:<id>:<timestamp>`), making message listing extremely efficient compared to full table scans or wide secondary queries.

### Small, predictable storage engine

ProgressDB uses Pebble, an embedded key-value store that delivers fast local I/O, automatic compaction, and minimal operational overhead. No heavyweight cluster coordination is required for single-node deployments.

### First-class chat features

Message versioning, edits, replies, reactions, and soft deletes are built-in primitives — providing auditability and consistency without extra application-level bookkeeping.

### Security-first by design

Optional field-level encryption is backed by a Key Management Service (KMS) and a signing flow that keeps backend keys isolated from client environments.

## How ProgressDB Achieves Performance

- **Timeline-optimized keys:** Data is stored in append-ordered keys, allowing efficient prefix scans and sequential reads instead of maintaining secondary indexes.
- **Compact object model:** Messages are JSON documents with minimal fixed metadata, avoiding costly joins and large deserialization overhead.
- **Lightweight runtime:** A single Go binary with tuned concurrency and Pebble optimizations minimizes CPU and memory usage — enabling fast cold starts and efficient single-node performance.

## Typical Use Cases

- Persistent chat history for AI assistants or LLM-driven agents
- Real-time chat for applications, communities, or support tools
- Conversation storage for analytics, search, or replay systems

## Trade-offs and Limitations

ProgressDB is designed for chat-centric workloads and excels on single-node or moderate-scale deployments. It’s not yet optimized for massive, globally distributed systems. For large-scale, multi-region, or strongly consistent clusters, keep an eye on future updates as ProgressDB evolves.