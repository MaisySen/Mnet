# Control-plane scale baseline

Measured locally on 2026-11-16. This is a repeatable engineering baseline,
not a production capacity claim or a distributed load test.

## Method

Synthetic state seeded directly with 100, 1,000 and 10,000 approved nodes
in one network. No real user accounts, no hosted-service traffic. The
profiles measure map construction, authenticated loopback HTTP map
responses with two benchmark workers, and persisted endpoint updates.
Registration, TLS, WAN latency, relay throughput, filesystem durability,
multi-process behavior and real-client fleet operation are not measured.
The JSON store's writes use temporary-file replacement but do not fsync.

Environment: Linux amd64, Intel Xeon Platinum 8375C @ 2.90GHz,
GOMAXPROCS=2. Each benchmark ran three times, at least 500ms per sample.
Complete command output is in benchmarks/control-2026-11-16.txt.

```
GOMAXPROCS=2 go test -run '^$' \
  -bench 'Benchmark(NetworkMap|MapHTTP|EndpointPersistence)Scale' \
  -benchmem -benchtime=500ms -count=3 ./internal/control
```

## Results

Observed mean time per operation across the three runs:

| Nodes in network | Build one map | HTTP map serving, amortized | Persist one endpoint update |
| --- | --- | --- | --- |
| 100 | 0.100-0.103ms | 0.266-0.282ms | 0.452-0.474ms |
| 1,000 | 1.563-1.585ms | 2.820-2.963ms | 3.975-4.110ms |
| 10,000 | 13.770-15.542ms | 28.987-34.152ms | 42.573-44.416ms |

The HTTP numbers are Go parallel-benchmark throughput timings, not
per-request latency percentiles. They must not be labeled p95 or p99.
Map construction at 10,000 nodes allocates about 11.1MB per operation;
HTTP responses allocate about 17.5-18.8MB per operation in this profile.

A separate race-enabled regression starts 1,000 simultaneous map watchers,
cancels them and verifies every watcher exits and no registrations remain.
The request handler now forwards HTTP cancellation into the store wait;
previously disconnected clients could remain registered for 50 seconds.
This is not evidence of 1,000 active VPN clients or a 10,000-device deployment.

## What it tells us

The current single-process JSON store is not a proven large-fleet design.
Maps scan the node table under one lock; endpoint updates rewrite the whole
store under that lock. At 10,000 nodes these are material CPU, allocation
and write bottlenecks even before WAN traffic and client fanout.

Before a production capacity target is agreed, add a sustained mixed-load
harness with connection churn, registration, endpoint updates, map fanout,
relay traffic, resource sampling and latency percentiles. Then evaluate
per-network indexes/cached immutable maps, durable indexed storage and
backpressure. Re-run against representative hardware and independent
clients. Do not infer a supported user count from these microbenchmarks.
