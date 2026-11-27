# DNS companion-port test fixture

The audit reproduced these Windows setup failures locally. The TCP ephemeral allocator issued consecutive ports covered by a separate Windows UDP exclusion. Readiness, the simulated upstream and bind-refusal checks now share a test allocator that samples across the dynamic range and owns both loopback sockets before use. It retries only bind collisions and Windows port exclusions, closes each partial reservation, and reports exhaustion after 32 candidates. The production resolver still uses its fixed-port admission path and checks the same query behavior.

The endpoint-filter checkpoint `d6f9659` has a preserved failed Windows job: [normal Windows job135653073551](https://github.com/MaisySen/Mnet/actions/runs/44550631128/job/135653073551). At the forwarded-DNS fallback step, `TestForwardedTCPFailureDoesNotPublishTruncatedUpstreamAnswer/closed` failed while starting its local fixture, before issuing a query or evaluating the product assertion. A TCP ephemeral port was allocated, then the companion UDP bind on that selected loopback port returned Windows WSAEACCES10013. This does not establish why the runner denied that port, a DNS product failure or an endpoint-filter failure.

The test-only allocator already retried address-in-use clashes, since UDP and TCP availability are independent. It now also rejects a candidate only when the companion UDP bind reports a nested `net.OpError` with operation `listen`, network `udp` and Windows socket error10013. It closes the TCP reservation before choosing a new candidate, within the same existing eight-attempt limit. Generic permission errors, TCP allocation denial, dial errors and bare error10013 are not classified as that companion-bind condition.

This is a fixture correction, not a production retry policy. No resolver code, query budget, response assertion, fixed-port binding behavior or application configuration changes. Exhaustion still fails the test rather than hiding the condition.

Two injected regressions fail against the old helper: recovery after a denied companion bind and bounded exhaustion. The corrected helper passes30 race repetitions, including listener closure and narrow error classification. Five repetitions of the forwarded-DNS family, full repository race tests, vet and source checks pass locally. A Windows test binary cross-compiles; that is not an actual Windows execution pass. The normal native selector now includes the allocator regressions without changing triggers or settings.

Fresh all-platform CI and deployment checks for this correction are pending. The original failed job remains part of the history. No blind rerun, installed-device/native acceptance, production deployment, Multies work, default change or app release is implied.

```sh
go test -race -count=30 -timeout=3m ./internal/dns -run '^TestEphemeralSetup'
go test -race -count=5 -timeout=2m ./internal/dns -run '^TestForwarded'
go test -race -count=1 -timeout=5m ./...
go vet ./...
```
