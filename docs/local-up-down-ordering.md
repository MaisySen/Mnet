# Local Up and Down ordering

The local API acknowledges an Up request before its background worker necessarily runs. Previously a completed Down could be followed by that older worker starting anyway. An authenticated-handler regression reproduces the order at the response-write boundary. Empty Up, routing reset and route replacement all failed the old behavior: the retired request could create an identity, rewrite routing or publish a startup error after Down completed.

Each accepted local Up now captures the current stop generation. Down retires the generation before waiting on the lifecycle lock. A worker that no longer owns its generation exits without routing, identity or error changes. Current workers serialize their routing and startup with other startup/teardown operations and check the generation again at startup admission. Cancellation is published with admission before configuration I/O, matching the existing preparation cancellation pattern.

Ordinary synchronous Up remains available, and a new local Up after Down still reaches normal startup. Down preserves saved identity and retains its existing error-display behavior. Close remains permanent. A separate correction rejects SetRouting after Close, including when a delayed local Up carries routing options, without changing ordinary stopped-node routing edits.

## Evidence and limits

- Three authenticated-handler cases failed the old ordering. The regression places Down before allowing the scheduled worker to run.
- A retired worker cannot replace the saved state or error from a fresh current attempt. A new generation still performs ordinary startup validation.
- A real local HTTP server and blocked loopback control fixture verify Down cancels an already-admitted enrollment and preserves the generated identity. This requires no device or external service.
- Closed-node replacement/reset failed direct and Unix-HTTP regressions against the old source, including showing that a fresh daemon inherited the unwanted saved change.
- Twenty focused race repetitions, full repository race tests, vet and source checks pass locally. Fresh hosted checks are separate and still required for this source.

There is no new endpoint, capability, routing default, device operation, Multies integration, production deployment or app release.

```sh
go test -race -count=20 -timeout=2m ./internal/daemon \
  -run '^TestLocalAPI|^TestLocalUp|^TestClose|^TestClosed|^TestDown|^TestPrepare|^TestUnsupportedRouting'
go test -race -count=1 -timeout=5m ./...
go vet ./...
```
