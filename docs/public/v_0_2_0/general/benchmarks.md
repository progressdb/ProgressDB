---
section: general
title: Benchmarks
order: 5
visibility: public
---

# Benchmarks (Preliminary, MacBook M2 Pro)

- The benchmark was run on a PC with active usage (code, editor, screen recorder etc).
- Both benchmarking tool & database service running on the same device.
- A dedicated instance (official benchmarks) is in the works.

---

## Benchmark Run 1

```
Pattern: thread_with_messages
Target RPS: 1000
Duration: 30s

Requests: 29128 | RPS: 987.4 | Avg: 776.166µs | Min: 361.458µs | Max: 52.772375ms

BENCHMARK RESULTS SUMMARY:
- Target vs Actual RPS: 987.6 → 987.6
- Total Requests: 29629 (29629 success, 0 failed)
- Latency: Avg 776.183µs | P90 752.209µs | P95 1.819917ms | P99 6.477834ms
- Data Sent: 1740.11 MB
- Duration: 30.000s
- Success rate: 100.0%
```

---

## Benchmark Run 2

```
Pattern: thread_with_messages
Target RPS: 2000
Duration: 20s

Requests: 38536 | RPS: 1926.8 | Avg: 968.431µs | Min: 344.459µs | Max: 54.891625ms

BENCHMARK RESULTS SUMMARY:
- Target vs Actual RPS: 1926.8 → 1926.8
- Total Requests: 38537 (38537 success, 0 failed)
- Latency: Avg 968.418µs | P90 1.359084ms | P95 2.243209ms | P99 8.670792ms
- Data Sent: 2263.28 MB
- Duration: 20.000s
- Success rate: 100.0%
```

---

## Benchmark Run 3

```
Pattern: thread_with_messages
Target RPS: 3000
Duration: 20s

Requests: 55237 | RPS: 2832.6 | Avg: 1.446278ms | Min: 348.417µs | Max: 62.04175ms

BENCHMARK RESULTS SUMMARY:
- Target vs Actual RPS: 2827.0 → 2827.0
- Total Requests: 56541 (56541 success, 0 failed)
- Latency: Avg 1.467311ms | P90 2.0785ms | P95 4.394083ms | P99 16.004ms
- Data Sent: 3320.66 MB
- Duration: 20.000s
- Success rate: 100.0%
```

---

_**Note:** All results above are local/Mac preliminary, single-node and not on server-grade hardware. Numbers are intended as a starting point for further tuning or scale-out benchmarks._