---
section: general
title: "Concepts"
order: 2
visibility: public
---

# Concepts

This page explains ProgressDB's core concepts so you can reason about the API
and design your application accordingly.

- Threads — a logical conversation or channel that contains messages.
- Messages — append-only items stored in a thread. Messages support versions,
  soft-deletes, replies, and reactions.
- Authors & Signing — clients authenticate users with a signature issued by a
  trusted backend; backend SDKs provide helpers to sign user IDs.
- Storage model — denormalized keys for thread timelines and message-version
  indices to make timeline reads and version lookups efficient.
- KMS & Encryption — optional per-thread encryption via a KMS provider; the
  server supports embedded or remote KMS for key wrapping/unwrapping.

Use these concepts when designing your data model, SDK usage, and operational
plans.

