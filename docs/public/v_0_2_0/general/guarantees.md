---
section: general
title: "Guarantees"
order: 3
visibility: public
---

# Guarantees

## Write Guarantees
- **Atomic**: Batched operations succeed or fail together
- **Consistent**: Global ordering, atomically written
- **Durable**: Full durability or fast mode (configurable)
- **Isolated**: Thread-scoped operations do not interfere with each other

## Read Guarantees
- **Consistent**: Point-in-time views
- Reads do not see partial or in-progress writes

## Performance
- **~1ms latency** for most writes (configurable)
- Optimized specifically for chat workloads
- Predictable, sub-millisecond response times

## Durability Modes
- **Fast Mode**: ~1ms loss window, ultra-fast
- **Durable Mode**: Full WAL, ~2ms extra latency per write
- Trade performance for durability based on your needs

## Chat-Optimized
Designed for chat patterns, not traditional SQL transactions. Tune the balance between speed and durability for your application.
