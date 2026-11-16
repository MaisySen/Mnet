# Mnet project status

Updated 2026-11-16, Australia/Sydney. Development/testing only. No production-readiness, parity, security-certification or supported fleet-size claim.

## Source, downloads and deployment

| Layer | Recorded state | Evidence / limit |
| --- | --- | --- |
| Published test artifacts | v0.8.2-test.1, source `2bb55f4be60969a855b37fbb6d607f0831542e45`, Go 1.26.11; Android 0.8.2/code 14. | [Release](https://github.com/MaisySen/Mnet/releases/tag/v0.8.2-test.1). Sixteen assets verified at release. Update CLI and daemon together. |
| Hosted test control plane | Last verified 2026-11-16 at 07:12 AEDT on `2bb55f4`. | Dated deployment check, not a current availability promise. Free-host ephemeral disk/restart state loss remains a limitation. |
| Newer verified source | `c2fe64c`, all five jobs passed. | [Exact CI](https://github.com/MaisySen/Mnet/actions/runs/43780929040). New source below is not in those downloads or that last verified deployment. |

## What is implemented and tested

### Mesh, routing and recovery

- Three independently keyed Linux kernel VPN devices in one network: unique IPs/keys, two peers per device, six directed mesh pings and third-device OS TCP 3 x 4MiB with receiver hash/sender ACK. Removing that device preserves the original mesh.
- A fresh device in a second network on the same control service sees no original peers, is absent from original maps, and cannot reach distinct original mesh IPs. Network address spaces can overlap; its own IP is excluded from this test.
- Bounded authenticated direct probes and filtering of mesh/advertised-prefix underlay candidates address reproduced convergence/recursive-routing hazards. Direct paths are asserted, not inferred from healthy relay traffic.
- Managed Linux RFC1918 IPv4 subnet routes and destination-scoped NAT have explicit ownership, rollback and cleanup. The separate LAN host has no mesh return route. Serving requires operator-enabled forwarding; Mnet never changes that shared sysctl. Unrelated NAT/routes and the operator LAN route are preserved. Older broad NAT rules are not adopted or deleted.
- With forwarding disabled, serving setup ends Down without a TUN/NAT leak or unrelated changes. Retry succeeds after the fixture enables forwarding. Saved Down/Up cycles and a fresh daemon process using saved state and `-up` restore identity, direct VPN and LAN access.
- Route-only ACL withdrawal/restoration and admin expiry/reapproval block and restore access as expected. Reapproval clears stale approval status/link. Known expiry is enforced locally even if later control polling is blocked; unseen offline admin changes cannot be discovered.
- `mnet up --reset-routing`, while down, clears saved roles without deleting identity. Ordinary saved-state Up preserves routing. Fresh attempts clear old setup errors, preserving errors raised during the new attempt.

The latest [c2fe64c CI](https://github.com/MaisySen/Mnet/actions/runs/43780929040) repeated the full kernel fixture three times. Later original-pair/LAN transfers ran with control and relay stopped: LAN TCP 3 x 4MiB, mesh normal TCP 3 x 4MiB and mesh TCP with 1% loss/20ms delay 3 x 4MiB, with receiver hash/sender ACK. Actual loss drops: 131/144/138. Third-device transfers occurred before control was stopped. These are disposable controlled fixtures, not fleet or public-internet soak proof.

Exit-node selection/advertising is explicitly rejected before enrollment/device creation. Unsafe dormant host/default-route replacement helpers were removed, not enabled. Exit internet routing remains unsupported.

### Platforms and sessions

- Windows: native protected-DACL/private-state, local API, expiry, preparation/cancellation and loopback OS TCP regressions. Actual SCM install/start/stop/restart, forced-crash recovery and removal pass on a disposable runner. No Wintun VPN or physical reboot claim.
- macOS: native build/private-state/local-API, preparation/cancellation, expiry and loopback sessions. Actual VPN, physical capture/input, consent and launchd boot remain unverified.
- Shared mobile lifecycle: Prepare reports preparing, serializes with Up/Down, publishes identity under a lock and is cancellable by Down. State snapshots do not share mutable route slices. Local race regressions and native CI pass, not physical mobile proof.
- Android: signed test artifacts and a prior virtual Firebase Test Lab join. Physical VPN/capture/input/reconnect and reboot startup remain open. iOS has source/shared Go coverage, not signed TestFlight or physical-device proof.
- Sessions: isolated X11 changed-frame/chat/hash/ACK and bounded stress/loss proofs. Production sessions use OS TCP; the attributed gVisor adapter is proof-only.

## Account groundwork, not a finished account product

Same-network multi-device connectivity is verified. A single login owning/managing all devices is not finished. Verified OIDC issuer/subject is stored as private approval provenance, separately from display email; no identity is inferred from historical email labels or disclosed in peer maps. Persistence, deletion, failed-write rollback and non-disclosure regressions pass in c2fe64c. The signed-provider callback assertion also passed all five jobs in [471f537 CI](https://github.com/MaisySen/Mnet/actions/runs/43781793743), with 149/143/176 later kernel loss-profile drops.

See [product scope](product-scope.md) for the user's account/device/startup target and platform gaps. The shared architecture blueprint is reference, not an adopted stack, budget or migration plan.

## Private identity-device query (new source, pending CI)

An internal helper can list approved, unexpired device summaries for an exact network + verified issuer + subject. It excludes same-email/different-subject identities, identical subjects at other issuers, other networks, legacy email-only approvals and expired devices. Returned summaries contain no keys/tokens/endpoints and do not alias stored state. Local race regressions cover persistence, scope, unknown inputs and expiry. All five jobs passed in [11198e8 CI](https://github.com/MaisySen/Mnet/actions/runs/43782589045), including three full kernel fixtures with 135/145/156 later loss-profile drops. No HTTP route exposes this helper; any future caller must authenticate the principal and separately authorize network access. This is not ownership, membership, a login UI or an authorization grant.

## Explicit network membership gate (new source, pending CI)

Private control-state membership now binds an exact network/issuer/subject tuple by an explicit internal operator/admin method. Verified approval alone never grants membership. A separate gated device query denies missing/revoked/wrong-network/wrong-issuer/wrong-subject membership and returns only that principal's approved, unexpired devices, not everyone in the network. Grant/revoke persist with rollback on save failure; tests cover restart, tuple collisions, unknown scope and no cross-principal disclosure. No HTTP route exposes grants or account queries. Member is the only access shape; owner/admin roles and a user-facing login remain unfinished. Revoking this membership only closes the account-query gate, not device transport credentials; explicit device expiry is still required to revoke mesh traffic. Independent CI is pending.

## Important open work

- Account/organization ownership, membership and management experience.
- Wider subnet/firewall compatibility, non-Linux/IPv6/public subnet OS routes and owned exit routing.
- Real Windows Wintun/macOS VPN, second-user access, physical capture/input/permissions, mobile and actual OS boot/reboot.
- Larger/cancelled transfers, long sessions, sleep/wake, sustained churn and fleet/relay capacity.
- Durable hosted storage, recovery practice, independent security review and store-policy/data-disclosure review. Store copy remains a draft.

Methods and earlier failures: [native validation](native-platform-validation.md), [reliability](session-reliability.md), [operations](operations.md), [release history](RELEASES.md) and [synthetic scale measurements](scale-testing.md).
