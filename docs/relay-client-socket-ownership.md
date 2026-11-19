# Relay client socket ownership

A relay reader owns its WebSocket and underlying TCP connection until it ends. Previously ReadMessage failures closed the public Done signal without closing the socket. The daemon reconnects after Done, so an oversized incoming packet or an ordinary remote Close could leave the old TCP connection retained while a new relay was opened.

The reader now closes its own WebSocket before publishing Done, on every exit. Explicit Close remains safe alongside reader exit and repeated Close calls. There is no change to relay identity, packet limits, authorization classification, callback framing, retry timing or protocol.

Two real loopback TCP/WebSocket regressions failed against the old source. In both the oversized-packet and remote-Close case, the peer timed out waiting for TCP EOF even after Done had fired. With the correction the peer observes closure. A separate healthy relay still exchanges a complete packet after another reader ends and is closed repeatedly; explicit healthy Close also ends its reader.

Twenty full control-client race repetitions, the full repository race suite, vet and source checks pass locally. Existing normal native CI already runs the entire control-client package; fresh hosted results for this correction remain separate and pending. These are loopback socket/resource checks, not public-internet/NAT reliability, physical reconnect, production deployment or app-release acceptance.

```sh
go test -race -count=20 -timeout=2m ./internal/controlclient
go test -race -count=1 -timeout=5m ./...
go vet ./...
```
