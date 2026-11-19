# Map endpoint filtering without repeated route rebuilding

Each received map filters peer UDP candidates that would recurse into the mesh or any advertised subnet. The old loop rebuilt and parsed the entire route list separately for every peer. With many routed peers, endpoint filtering alone could allocate gigabytes and delay application of a current map. This was a client map-application cost, separate from the control-store scale measurements.

The current loop builds one map-scoped prefix index. For each candidate it probes only the prefix lengths present for that address family, at most 33 IPv4 or 129 IPv6 lengths. The index is rebuilt for each map application, not cached across map versions. Retired routes therefore cannot keep blocking fresh candidates, and added routes take effect in the current application.

The safety policy is unchanged: the mesh prefix and advertised subnets block recursive candidates; malformed candidates are ignored; original candidate ordering and duplicates remain intact. IPv4-mapped and scoped IPv6 handling matches the original `netip.Prefix.Contains` behavior. This change does not grant new routes or change exit-node, relay or path-selection defaults.

## Local comparison

Linux amd64, Intel Xeon 6975P-C. Three one-iteration samples per case. Each synthetic peer advertises one distinct IPv4 /24 and two candidates, one safe and one inside a blocked route. Both implementations must retain the same total candidates. The comparison includes index construction once per batch. The original-policy reference is retained in tests so the same command runs both implementations in one process.

| Routed peers | Original batch time | Indexed batch time | Original bytes allocated | Indexed bytes allocated |
| --- | --- | --- | --- | --- |
| 100 | 0.450-0.795ms | 0.022-0.038ms | 929,600 | 12,344 |
| 1,000 | 51.02-52.10ms | 0.230-0.279ms | 70,224,000 | 207,016 |
| 10,000 | 8.367-8.459s | 3.257-3.505ms | 14,710,560,000 | 1,728,392 |

[Raw command output](benchmarks/endpoint-filter-2026-11-20.txt). These are short synthetic filter-batch measurements, not full `applyMap` timings, peak live memory, percentiles, WireGuard configuration performance, VPN throughput or production fleet capacity. The original 14.7GB number is cumulative allocation during one batch, not a claim that all those bytes are live simultaneously.

## Regression coverage and limits

An independent copy of the old linear rule is compared against the index for every IPv4/IPv6 prefix length, duplicate and malformed routes, non-masked routes, overlapping families, mapped/scoped addresses, mesh boundaries and malformed endpoints. A seeded mixed-address corpus adds repeatable coverage. Input candidates remain unchanged. An actual in-memory WireGuard daemon map test checks that a different peer's route blocks a direct candidate, route replacement permits it again, and restoring the earlier map blocks it again.

Twenty focused race repetitions and the full repository race suite, vet and source checks pass locally. The existing normal native CI selector includes the regression families without changing triggers or settings. Hosted checks for this source remain pending. No installed-device/native acceptance, production deployment, Multies work, file-entry/reception enablement or app release is implied.

```sh
go test -race -count=20 -timeout=2m ./internal/daemon -run '^TestEndpointRouteFilter|^TestCandidatesCannot|^TestApplyMapRoutes|^TestPath'
go test ./internal/daemon -run '^$' -bench '^BenchmarkMapEndpointFiltering$' -benchmem -benchtime=1x -count=3
go test -race -count=1 -timeout=5m ./...
go vet ./...
```
