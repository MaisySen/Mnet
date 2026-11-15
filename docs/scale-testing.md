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

## Short sustained mixed-load profile

The opt-in TestControlMixedLoad harness keeps 16 HTTP workers active for
10 seconds against an isolated loopback server. The synthetic initial
state contains either 1,000 or 10,000 nodes. Each worker uses its own seeded
node identity (up to 16 identities), with a repeatable request mix:
70% map reads, 20% changing endpoint updates, 5% registration attempts and
5% cancellable map polls. One percent of requests close their connection.
Registration uses the normal server admission limit, not a disabled test
limiter. A short separate race-enabled execution checks the harness.

```
GOMAXPROCS=2 MNET_MIXED_LOAD=1 MNET_LOAD_NODES=1000 \
  MNET_LOAD_OUTPUT=/tmp/mixed-load.json \
  go test -count=1 -run '^TestControlMixedLoad$' -timeout=20s -v ./internal/control
```

Set MNET_LOAD_NODES=10000 for the larger profile; MNET_LOAD_DURATION can
select a duration from 1s to 1m. The timing runs do not use the race detector.
Raw measured summaries are in benchmarks/mixed-load-*-2026-11-16.json.

| Initial seeded nodes | Completed requests/sec | Map p95 | Endpoint-update p95 |
| --- | --- | --- | --- |
| 1,000 | 307.4 | 81.7ms | 61.1ms |
| 10,000 | 30.9 | 738.7ms | 550.4ms |

No unexpected HTTP or transport errors occurred in these runs. This is
not the same as every offered operation succeeding: the normal per-IP
registration limiter rejected most registration attempts in the 1,000-node
run, and polls deliberately use a 100ms cancellation deadline. The
10,000-node profile cancelled 16 of its 17 polls at that deadline. The raw summaries
separate OK, rate-limited, cancelled and unexpected outcomes per operation.
Percentiles combine all attempts within each operation, so join percentiles
include fast rate-limit responses, and poll percentiles include cancellation.
These are observed closed-loop latencies, not an open-loop overload/SLO test.

The elapsed time includes draining in-flight requests. Allocated MB is
cumulative allocation, not resident memory; the end heap measurement is
not a peak. Goroutine counts after worker drain still include HTTP keepalive
connections and server work, so they must not be read as leak counts.
The harness checks that map-watcher registrations return to zero.

This short test is local and synthetic, with one process and two Go
workers. It is not a soak test, independent-client validation or a certified
fleet size. It gives stronger evidence for the store bottleneck and a
repeatable way to measure changes before choosing a production architecture.
