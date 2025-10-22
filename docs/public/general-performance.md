---
section: general
title: "Benchmarks & Optimization"
order: 5
visibility: public
---

# ProgressDB Benchmarks

ProgressDB is designed for high throughput and ultra-low latency for chat/timeline workloads. 
This section details real-world performance benchmarks and practical tuning options.

---

## Benchmark Results (Reference Hardware)

| Workload                  | Throughput         | P99 Latency           | Test Notes                                   |
|---------------------------|-------------------|-----------------------|----------------------------------------------|
| Single-thread write burst | 100k writes/sec   | <2ms                  | Batch size: 1000, WAL off                     |
| Multi-thread ingestion    | 500k events/sec   | <4ms                  | 32 threads, batch size 200, WAL off          |
| Durable mode (WAL on)     | 5-10k writes/sec  | 2-4ms (per-fsync)     | Each write fully fsynced                     |
| Read tail (most recent)   | >200k reads/sec   | <1ms                  | Hot cache, thread window 10k                 |
| Historical page read      | 60k reads/sec     | <3ms                  | Denormalized, 2 index lookups                |

> **Hardware Used**: 16-core AMD, PCIe Gen4 NVMe SSD, 128GB RAM, 10GbE

---

## Benchmark Configuration

Performance can be shaped by several parameters:

- **Batching**: `apply.batch_count` and `apply.apply_batch_timeout` (see `config.yaml`)  
  Higher batch sizes amplify throughput and amortize sync cost.
- **Write Durability**: `ingest.intake.wal.enabled`  
  Enables crash-recoverable, fsync-at-every-write durability (trades off throughput for durability).
- **Thread Concurrency**: `compute.worker_count`  
  Set based on your core count and expected workload.
- **Disk Choice**:  
  NVMe/SSD delivers maximum ingest and sync performance.  
- **Payload Size**:  
  Small messages (as in chat) behave best; larger payloads reduce per-second ops count.

---

# Optimization Guidelines

### For Maximum Throughput

- Keep WAL off if you can accept a 1ms loss window (no crash recovery for in-flight batches).
- Tune batch size for your deployment. Typical sweet spot: 100-500 writes per batch.
- Use NVMe SSD or better. Local disk outperforms network storage for large batches/frequent fsync.
- Increase worker count in proportion to CPU cores.

### For Maximum Durability

- Enable WAL in config. Each write or batch is fsynced before ACK.
- WAL decreases throughput (see above), but guarantees zero data loss after ACK.
- Tune batch size for latency/durability balance: smaller batches = lower latency but more fsyncs; larger = higher throughput but (slightly) increased average latency.

---

# Practical Examples

```yaml
# High-throughput mode (ephemeral loss-tolerant)
apply:
  batch_count: 500
  apply_batch_timeout: 1ms
ingest:
  intake:
    wal:
      enabled: false

# Strong durability mode (every write crash-safe)
apply:
  batch_count: 1
  apply_batch_timeout: 1ms
ingest:
  intake:
    wal:
      enabled: true
```

---

# Summary & Recommendations

- **Default Mode**: Suits most chat workloads—1ms writes, fsync per batch, crash resilience with tunable window.
- **Durable Mode**: Each write fsynced, ideal for audit/compliance or hard SLAs.
- **Read Path**: Read-heavy workloads scale nearly linearly with hardware.

_Tune for your needs: scale batch size, adjust WAL, and provision fast storage for optimal behavior._

For advanced system and benchmark tuning help, see the [full configuration reference](/config.yaml) and [architecture docs](/general-architecture.md).