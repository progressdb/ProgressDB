---
section: general
title: "Welcome"
order: 1
visibility: public
---

<!-- ProgressDB Logo -->

# Welcome to ProgressDB

ProgressDB is a chat-native database built for storing and serving conversational data — threads, messages, and related context — with high efficiency and predictable behavior.

ProgressDB is optimized for the patterns of chat systems: append-only timelines, fast ordered reads, message versioning, and field-level encryption for sensitive content.

## Why ProgressDB

### Purpose-built indexing

Keys are organized for append-ordered timeline access, making message listing extremely efficient compared to full table scans or wide secondary queries.

### Small, predictable storage engine

ProgressDB uses Pebble, an embedded key-value store that delivers fast local I/O, automatic compaction, and minimal operational overhead. No heavyweight cluster coordination is required for single-node deployments.

### First-class chat features

Message versioning, edits, replies, reactions, and soft deletes are built-in primitives — providing auditability and consistency without extra application-level bookkeeping.

### Security-first by design

Optional field-level encryption is backed by a Key Management Service (KMS) and a signing flow that keeps backend keys isolated from client environments.

### Built-in performance
Append-optimized storage, compact data structures, and a tuned Go runtime deliver ultra-fast reads, instant startups, and efficient operation — all without complex infrastructure or tuning.


## Typical Use Cases

- Persistent chat history for AI assistants or LLM-driven agents
- Real-time chat for applications, communities, or support tools
- Conversation storage for analytics, search, or replay systems


## Trade-offs and Limitations

ProgressDB is purpose-built for chat-centric workloads and performs best in single-node or moderate-scale deployments. It’s not yet optimized for massive, globally distributed systems. For large-scale, multi-region, or strongly consistent cluster needs, reach out at henry@progressdb.dev