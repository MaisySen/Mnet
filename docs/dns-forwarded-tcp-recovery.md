# Forwarded DNS truncation and TCP recovery

This source correction uses the existing shared DNS resolver and upstream choice. It does not change OS DNS settings, the selected upstream, local Mnet names, native entry points or defaults. No app or control-service deployment is included.

## Failure reproduced

The resolver previously forwarded every external query with a UDP-only library call. That call deliberately does not retry over TCP when an upstream response has its truncation flag set. A local client retrying the Mnet resolver over TCP therefore received the same incomplete UDP response again.

An actual loopback UDP/TCP upstream fixture reproduced the failure through both real local resolver transports: both returned a truncated empty answer, and the upstream TCP handler was never called. The fixture's complete TXT answer is larger than a legacy UDP response.

## Correction

The shared resolver now retries one truncated upstream UDP answer over TCP. The UDP query and TCP retry share one two-second overall query budget. An upstream TCP failure, stall or still-truncated TCP answer produces SERVFAIL without publishing incomplete data.

The complete answer is preserved for local TCP callers. For local UDP callers, the response is sized to the request's EDNS buffer or the legacy 512-byte minimum, with a proper truncation flag when records do not fit. This lets a client retry locally over TCP instead of receiving an oversized datagram. Complete UDP answers, including an upstream NXDOMAIN, do not open an unnecessary TCP connection.

## Verification

Twenty race-enabled repetitions cover real UDP/TCP forwarding, complete large TXT data, request ID/question preservation, legacy UDP sizing, no unnecessary fallback, and closed/still-truncated TCP failure. A separate delayed-UDP/stalled-TCP fixture checks the shared deadline rather than allowing independent full budgets for each transport. The full repository race suite, vet and tracked-source checks pass locally.

These are real local socket and resolver checks, not evidence from an installed phone, OS default resolver, physical network, Android native matrix or production upstream. Those acceptance gates remain separate. Hosted CI outcomes are checked independently.

```sh
go test -race -count=20 -timeout=2m ./internal/dns \
  -run '^TestForwardedTruncated|^TestForwardedComplete|^TestForwardedTCPFailure'
go test -race -count=1 -timeout=30s ./internal/dns \
  -run '^TestForwardedStalled'
go test -race -count=1 -timeout=5m ./...
go vet ./...
sh scripts/check-secrets.sh
```
