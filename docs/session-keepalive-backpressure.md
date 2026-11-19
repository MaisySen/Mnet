# Keepalive under outbound backpressure

The session watchdog checks how long it has been since any inbound frame and periodically queues a ping. Previously, a full shared outbound queue could block that same watchdog inside ping enqueue. If the socket writer was also blocked on a peer, no later tick could enforce the dead-peer timeout. The session could keep appearing open indefinitely instead of ending after loss of inbound liveness.

A real socket/enqueue regression reproduces this against the old source: block a `net.Pipe` writer on the peer, fill the existing bounded queue through ordinary enqueue, then start the watchdog. The old implementation misses the deadline because its ping waits for capacity.

The watchdog now attempts each ping without waiting for queue capacity. A skipped probe does not close the session. Later ticks still check the same inbound-liveness timeout, and probes resume when capacity returns. No queue limit, application-frame backpressure, timeout, wire format, consent or control-grant rule changes.

Tests require dead-peer closure under a full queue, continued session liveness while inbound traffic arrives despite blocked outgoing pings, no queue growth from skipped probes, and resumed ping/pong RTT measurement after the real socket backlog drains. Existing live/dead/RTT checks are included in20 race-enabled repetitions. Full repository race tests, vet and source checks pass locally. The normal native selector includes these tests without changing workflow triggers or settings. Fresh hosted verification is pending.

This corrects one watchdog capacity wait. It does not add general per-stream flow control, interrupt a blocked application callback, guarantee arbitrary loss/sleep/wake recovery or prove physical-device behavior. No Multies work, production service change, routing/default change, file-entry/reception enablement or app release is implied.

```sh
go test -race -count=20 -timeout=2m ./internal/session -run '^TestKeepalive'
go test -race -count=1 -timeout=5m ./...
go vet ./...
```
