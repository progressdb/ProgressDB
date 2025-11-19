---
section: general
title: "Concepts"
order: 2
visibility: public
---

# Concepts — design, benefits, and trade‑offs

A quick overview of ProgressDB’s core design principles and trade-offs.

## Design principles

### Append‑only timeline

- Core idea: all user events are stored as new entries rather than mutating existing records. Threads therefore behave like immutable logs with new events appended to the end.
- Benefit: chronological integrity, simpler concurrency (no in‑place races), and a natural audit trail.

### Thread‑first model

- Core idea: a thread is the primary unit of organization. Everything — messages, versions, metadata — is scoped to a thread.
- Benefit: client interactions (infinite scroll, tailing, per‑thread permissions) map directly to server semantics.

### Read‑optimized denormalization

- Core idea: optimize common read patterns (recent messages, tailing) even if it means storing small, deliberate duplicates to make reads cheap.
- Benefit: low latency for chat UIs, predictable CPU/IO during reads, and simpler client code.

### Versioning for auditability

- Core idea: edits, deletes, and similar state changes append versioned entries instead of overwriting. Historical states are preserved.
- Benefit: straightforward edit history, easier moderation and incident investigation, and safe soft‑delete semantics.

### Minimal, predictable APIs

- Core idea: keep the surface area small — thread/message primitives, lightweight signing, health and metrics. Clients should not need to reason about complex server internals.
- Benefit: easier SDKs, fewer breaking changes, and clearer expectations for integrators.

### Encryption by design

- Core idea: support encrypting sensitive fields or full payloads and separate key management so operators can choose embedded or external KMS setups.
- Benefit: flexible security postures — simple local encryption for dev, hardened external KMS for production.

## How these principles help users

- **Low latency UIs:** append‑only timelines and read‑focused structures let the server serve recent messages with minimal IO and CPU work — the result is snappy scroll and live tail behavior for end users.
- **Predictable ordering & consistency:** since events are appended, clients can rely on stable ordering and reason about causality (helpful for optimistic updates and conflict handling).
- **Auditability & compliance:** versioned history provides a complete trail without bespoke logging systems — useful for moderation, legal holds, and debugging.
- **Safer, incremental migrations:** monotonic data models make it easier to run background migrations or rewrap operations without blocking the service.
- **Clear security boundaries:** separating signing and KMS concerns reduces the risk surface and makes security reviews and hardening simpler.

## Performance and operational trade‑offs

### Read vs write cost

- Trade: optimizing reads can add modest write overhead (extra indexed entries, stored versions).
- Rationale: for chat workloads, reads are often much more frequent and latency‑sensitive than writes.

### Storage overhead & retention

- Trade: keeping history and denormalized entries increases disk usage.
- Mitigation: use retention policies, compaction, or TTLs to bound growth while preserving required audit windows.

### Compaction, retention and operational work

- Challenge: retained history requires operational processes (retention policies, backups, tested restores).
- Recommendation: plan retention windows that balance audit needs with storage cost, and automate restores for validation.

## Developer & contributor guidance

- Think in events: prefer appending descriptive events (edits, reactions, moderation markers) over in‑place mutations.
- Keep features thread‑scoped: map UI components and permissions to per‑thread behavior for consistency.
- Treat encryption and signing as separate, testable subsystems — keep secrets and key material out of client code.

## Common usage patterns

- **Chat & collaboration:** immediate append semantics make tailing and consistent scrollback trivial to implement.
- **Moderation & auditing:** version history simplifies change review and moderation workflows.
- **Analytics & export:** event streams are natural to replay for analytics, backups, or importing into other systems.

## When this design is not a fit

- Not ideal for workloads that demand tiny storage footprint, strictly mutable single‑record semantics, or where historical retention is actively harmful.
