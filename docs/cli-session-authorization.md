# Terminal session peer binding

## Reproduced gap

The terminal viewer and file sender resolved a display-status name/IP/FQDN, dialed it and accepted the session reply without verifying the host label. Reconnect repeated that display lookup and could follow a changed identity. Active sessions did not recheck current daemon authorization.

A real local TCP regression introduced an authorization adapter initially delegated to the unchanged handshake. A host returned a label unrelated to the selected current peer and was admitted. That regression failed before the implementation and passes after it.

## Implementation

- A new GET-only `/localapi/session-peers` route returns the existing approved/unexpired daemon snapshot, including canonical full public keys. It is behind the same private local capability as daemon control. Browser-origin requests are refused, responses are no-store, and ordinary display Status still has only short peer keys. This is not a public control-server route or an account permission API.
- CLI selection requires exactly one display selector match and a matching unique name/source/canonical full key in the current snapshot. A stale display name cannot authorize another peer that reused its address.
- Pin that canonical name, address and full key for the entire terminal connect/reconnect run, including one-shot file sending. Reconnect does not re-resolve the user's label to a new key or address.
- Check the actual TCP destination before hello. Verify the returned host label against current evidence after potentially blocking host consent and before mux startup. Wrong-label replies are terminal identity failures, not retryable reassignment.
- Check current local evidence before sends and redial. While connected, check the authenticated local daemon once per second, with a two-second request context. Known removal, key/name/address change, ambiguity or unavailable evidence ends that CLI run. Reapproval cannot revive it; the user must select again.
- Suppress subsequent frame-save/chat callbacks after a denial is observed. A transport-only failure can still reconnect to the same pinned authorized peer.

This closes the standalone CLI entry point. It does not add Multies identity, workspace roles, separate control consent or file approval.

## Compatibility and verification

The CLI now requires a daemon with the private session-evidence route. An older daemon's missing route fails closed; there is no fallback to display Status. The local API's capability file, listener binding and ordinary status/control behavior are unchanged. Full public keys are identity evidence, not private WireGuard keys, but the snapshot still contains private peer relationships and must not be published.

Fresh race tests cover wrong reply labels, removal/key change during consent, active removal/key change, local-evidence failure, terminal invalidation, stable valid peers, duplicate selectors, stale display names, and name/IP/FQDN selection. Evidence-response tests cover missing/denied routes, redirects, malformed/trailing JSON and the 4 MiB response cap. Local API tests verify actual route registration, authentication, browser refusal, GET-only behavior, no-store and approval/expiry filtering. Existing timeout/ownership fixtures retain an unverified handshake only in test code.

Fresh full Go race execution and go vet pass locally. General CI must be collected for the committed revision. Native Android diagnostics remain closed and are not evidence for this change.

## Limits

The daemon can return only authorization it has learned. Unknown offline administrative changes remain unknown. The one-second cadence and two-second request cap are implementation budgets, not a hard revocation guarantee. Frames, chat or file bytes can flow between observations; callbacks already dispatched and writes already started may finish. Saved snapshots and completed files are not erased on denial. A file already hashing locally may finish that work even after transport closure. Fail-closed local API failure may end an otherwise healthy session; start a new CLI run after checking daemon health and peer identity.

The terminal's existing combined headless host admission is not the consumer separate-consent design. No physical-platform, public-network, scale or production-readiness claim follows. Android DNS/bulk/reconnect reliability remains unresolved; Multies source/policy and release artifacts are unchanged.
