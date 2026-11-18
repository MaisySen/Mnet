# Outgoing mobile session authorization

## Defect reproduced

The production mobile viewer previously checked local mesh Up, dialed the supplied destination IP, then called the unverified session connector. The session protocol parsed but discarded the host reply's node label. Neither admission nor the active viewer checked the host against the current daemon peer map.

A real local TCP regression exercised a new adapter initially delegated directly to that unchanged connector. With an empty current peer map it still admitted a host. The regression failed before the authorization implementation. The adapter is now the verified production path; the old unverified mobile convenience connector exists only in lifecycle test fixtures.

## Repair

- Before dialing, require an IPv4 destination belonging to one uniquely named/source-matched peer with a canonical full key in the current daemon snapshot. Pin that name and key before connection setup. A hostname or arbitrary non-peer IP is no longer accepted by the public mobile method.
- Validate the actual TCP remote address before sending the session hello. The post-dial map must still match the originally pinned destination.
- Add an optional protocol-level reply verifier that runs before mux startup. The mobile connector requires one. Its host reply label must match the source-bound pinned map identity, and the name/key must still be authorized after host consent returns.
- Guard viewer input, chat and file-send entry points and frame/chat callback dispatch against known authorization loss. Once a denial is observed, restoring the peer map cannot revive the old session.
- Reuse the local-map watcher from host-side authorization. It closes the active viewer transport on known loss even when there are no new frames or user actions. This reads in-process state at the existing screen cadence, not an external server or website.
- Attach the mux to the shared guard under a lock because queued callbacks may observe denial before viewer setup returns. Such a denial still closes the newly attached mux.

The wire format and existing CLI Dial behavior are unchanged. This is standalone encrypted-mesh peer binding, not a Multies person/workspace verifier or a new consent model.

## Verification

Fresh loopback/race tests cover an absent map, wrong host reply label, removal/key change during consent, active removal/key change, a stable valid peer, blocked sends after denial, suppression of frame/chat callbacks, one viewer-close callback and terminal denial. Existing viewer timeout, owner Stop and pending Close regressions remain passing. A protocol regression checks verifier inputs, error propagation and owned transport closure before publishing a mux.

A fresh full Go race suite and go vet pass locally. General CI must be collected for the committed revision before reporting remote validation. No Android native reliability claim follows from these checks.

## Limits

Peer authentication relies on the encrypted mesh's source-address/key binding and the daemon's currently known authorization. There is no separate cryptographic host signature in the session hello, no proof of a Multies person, and no new offline freshness/grace-period policy. Unknown offline administrative changes cannot be enforced until learned.

A callback or input already dispatched may finish. A transfer already in progress is interrupted by transport closure, not atomically rolled back. A previously rendered frame is not erased by this Go guard; the app's existing close handling owns clearing its UI. Timer scheduling is not a hard revocation SLA. Existing Android DNS/bulk/reconnect reliability and physical lifecycle gates remain unresolved. Multies source, release artifacts, roles and grants are unchanged.
