# Mnet project status

Updated 2026-11-16, Australia/Sydney. Development/testing only. No production-readiness, parity, security-certification or supported fleet-size claim.

## Source, downloads and deployment

| Layer | Recorded state | Evidence / limit |
| --- | --- | --- |
| Published test artifacts | v0.8.2-test.1, source `2bb55f4be60969a855b37fbb6d607f0831542e45`, Go 1.26.11; Android 0.8.2/code 14. | [Release](https://github.com/MaisySen/Mnet/releases/tag/v0.8.2-test.1). Sixteen assets verified at release. Update CLI and daemon together. |
| Hosted test control plane | Last verified 2026-11-16 at 07:12 AEDT on `2bb55f4`. | Dated deployment check, not a current availability promise. Free-host ephemeral disk/restart state loss remains a limitation. |
| Newer verified source | `a08a67c`, all five jobs passed. | [Exact CI](https://github.com/MaisySen/Mnet/actions/runs/43789815773). New source below is not in those downloads or that last verified deployment. |

## What is implemented and tested

### Mesh, routing and recovery

- Three independently keyed Linux kernel VPN devices in one network: unique IPs/keys, two peers per device, six directed mesh pings and third-device OS TCP 3 x 4MiB with receiver hash/sender ACK. Removing that device preserves the original mesh.
- A fresh device in a second network on the same control service sees no original peers, is absent from original maps, and cannot reach distinct original mesh IPs. Network address spaces can overlap; its own IP is excluded from this test.
- Bounded authenticated direct probes and filtering of mesh/advertised-prefix underlay candidates address reproduced convergence/recursive-routing hazards. Direct paths are asserted, not inferred from healthy relay traffic.
- Managed Linux RFC1918 IPv4 subnet routes and destination-scoped NAT have explicit ownership, rollback and cleanup. The separate LAN host has no mesh return route. Serving requires operator-enabled forwarding; Mnet never changes that shared sysctl. Unrelated NAT/routes and the operator LAN route are preserved. Older broad NAT rules are not adopted or deleted.
- With forwarding disabled, serving setup ends Down without a TUN/NAT leak or unrelated changes. Retry succeeds after the fixture enables forwarding. Saved Down/Up cycles and a fresh daemon process using saved state and `-up` restore identity, direct VPN and LAN access.
- Route-only ACL withdrawal/restoration and admin expiry/reapproval block and restore access as expected. Reapproval clears stale approval status/link. Known expiry is enforced locally even if later control polling is blocked; unseen offline admin changes cannot be discovered.
- `mnet up --reset-routing`, while down, clears saved roles without deleting identity. Ordinary saved-state Up preserves routing. Fresh attempts clear old setup errors, preserving errors raised during the new attempt.

The latest verified [a08a67c CI](https://github.com/MaisySen/Mnet/actions/runs/43789815773) repeated the full kernel fixture three times. Later original-pair/LAN transfers ran with control and relay stopped: LAN TCP 3 x 4MiB, mesh normal TCP 3 x 4MiB and mesh TCP with 1% loss/20ms delay 3 x 4MiB, with receiver hash/sender ACK. Actual loss drops: 138/134/139. Third-device transfers occurred before control was stopped. These are disposable controlled fixtures, not fleet or public-internet soak proof.

Exit-node selection/advertising is explicitly rejected before enrollment/device creation. Unsafe dormant host/default-route replacement helpers were removed, not enabled. Exit internet routing remains unsupported.

### Platforms and sessions

- Windows: native protected-DACL/private-state, local API, expiry, preparation/cancellation and loopback OS TCP regressions. Actual SCM install/start/stop/restart, forced-crash recovery and removal pass on a disposable runner. No Wintun VPN or physical reboot claim.
- macOS: native build/private-state/local-API, preparation/cancellation, expiry and loopback sessions. Actual VPN, physical capture/input, consent and launchd boot remain unverified.
- Shared mobile lifecycle: Prepare reports preparing, serializes with Up/Down, publishes identity under a lock and is cancellable by Down. State snapshots do not share mutable route slices. Local race regressions and native CI pass, not physical mobile proof.
- Android: signed test artifacts and a prior virtual Firebase Test Lab join. Physical VPN/capture/input/reconnect and reboot startup remain open. iOS has source/shared Go coverage, not signed TestFlight or physical-device proof.
- Sessions: isolated X11 changed-frame/chat/hash/ACK and bounded stress/loss proofs. Production sessions use OS TCP; the attributed gVisor adapter is proof-only.

## Account and operator access

Same-network multi-device connectivity is verified. A single login owning/managing all devices is not finished.

- Verified OIDC issuer/subject is stored as private approval provenance, separately from display email. Historical email-only labels remain unknown; principals are not added to peer maps. An internal own-device query returns only approved, unexpired summaries for an exact network + issuer + subject. Explicit membership is required by its gated entry point; approval alone never grants membership. There is no HTTP account-device query, self-service account login or owner/admin role model. [Provenance CI](https://github.com/MaisySen/Mnet/actions/runs/43780929040), [signed callback CI](https://github.com/MaisySen/Mnet/actions/runs/43781793743), [query CI](https://github.com/MaisySen/Mnet/actions/runs/43782589045), [membership CI](https://github.com/MaisySen/Mnet/actions/runs/43783449695).
- `POST /admin/api/memberships` lets an authenticated operator explicitly grant/revoke an exact principal. A strict 4KiB body requires network/issuer/subject and an explicit boolean decision. The target principal is not caller authentication and must not be guessed from email. Persistence rollback and snapshot/audit non-disclosure are tested. Revocation closes this account-query gate only, not device credentials or mesh traffic. [Endpoint CI](https://github.com/MaisySen/Mnet/actions/runs/43784350674).
- Login and every admin mutation require JSON plus configured same-origin browser checks. State/metrics use GET, ACL uses GET/PUT, other mutations use POST only. Headerless non-browser JSON scripts remain supported. HTTPS admin cookies are Secure, HttpOnly and SameSite=Lax. [Admin request guard CI](https://github.com/MaisySen/Mnet/actions/runs/43785399011).
- Pending OIDC state is capped at 1,024 entries, reclaims expired entries and rejects full capacity without evicting live flows. States bind the starting provider, expire at equality and are one-use. Wrong methods cannot allocate/consume state. [OIDC state CI](https://github.com/MaisySen/Mnet/actions/runs/43786272564).


## Bounded admin sessions and logout (verified source)

The in-memory admin session table is capped at 1,024 entries. Successful password checks reclaim expired entries before allocation; a full live table rejects a new session with 429 rather than evicting active users. Signing in with an existing valid session rotates that token without increasing the table size, and the old token stops working. `POST /admin/api/logout` requires the same session/JSON/origin guard as other mutations, deletes the server-side token and clears the protected cookie. Replaying the old cookie is unauthorized. Expired authorization lookups also remove the stale entry. Local race regressions cover capacity/reclamation, rotation at capacity, failed-password non-revocation, cross-site logout refusal, cookie clearing and old-token replay. All five jobs passed in [9460300 CI](https://github.com/MaisySen/Mnet/actions/runs/43787156366), including three complete kernel fixtures with 153/123/137 later loss-profile drops. Sessions remain process-local and disappear on restart; logout does not revoke device credentials or change mesh transport.

See [product scope](product-scope.md) for the account/device/startup target and platform gaps. The shared architecture blueprint is reference, not an adopted stack, budget or migration plan. Detailed methods and earlier failures remain in the guides below.

## Dashboard sign-out and real-browser regression (verified source)

The operator dashboard explicitly says exit routing is unsupported, and has Sign out, clears rendered private state after logout and ignores a delayed pre-logout state/ACL response instead of reopening the dashboard. A CSS specificity bug that left the sign-in form visible while signed in is fixed. An isolated real-Chrome fixture runs the actual Go control service with disposable state/profile: signs in, sees its fixture network, signs out, releases an intentionally delayed valid refresh and checks the private UI stays cleared. It also verifies cookie removal and 401 when the old token is replayed. The local fixture passed and signed-in/signed-out screenshots were visually inspected. All five jobs passed in [0c61bcc CI](https://github.com/MaisySen/Mnet/actions/runs/43788090621), including the real-Chrome fixture, corrected unsupported-exit help and three full kernel fixtures with 166/146/156 later loss-profile drops. It uses no live account, user profile or hosted control state and is desktop Chrome proof only.

## Dashboard expiry truth and populated cleanup (verified source)

Key and device expiry display now distinguishes positive time remaining, expired at/before the deadline, no expiry and an invalid timestamp. It no longer converts a future timestamp into a negative "seconds left" value. The new-key dialog no longer claims the credential is shown only once while the admin table still exposes it. The real-Chrome fixture now creates an actual one-hour enrollment key through the dashboard, verifies the populated expiry row and checks exact formatter boundaries. It then signs out with that private row populated and releases the held old state response, asserting the key table and network selectors stay empty. No fixture credentials are printed or captured in screenshots. All five jobs passed in [10c8c1c CI](https://github.com/MaisySen/Mnet/actions/runs/43788935254), including the populated real-Chrome fixture and three full kernel fixtures with 155/158/132 later loss-profile drops. Admin access to stored keys remains an explicit trust boundary, not a one-time-secret design.

## Node-scoped relay presence (verified source)

The admin state now adds `online_by_node`, keyed by node ID rather than public key. The dashboard uses this map and calls the column "Relay connection", with "connected" / "not connected" labels. A missing relay connection is not proof that a direct-connected device is offline. Identical keys in different networks can no longer make another node appear connected or collapse the current relay node count. The legacy public-key `online` map remains for compatibility; it is not a network-scoped presence source. Real WebSocket regressions connect two same-key nodes in different networks, inspect authenticated state/count, disconnect them independently and check snapshot copy safety. All five jobs passed in [a08a67c CI](https://github.com/MaisySen/Mnet/actions/runs/43789815773), including the real-Chrome fixture and three full kernel fixtures with 138/134/139 later loss-profile drops.

## Bounded multi-device collision names (new source, pending CI)

Repeated device names now keep the automatic numeric suffix inside the same 32-character validated name bound. A maximum-length request previously grew beyond that bound when another device already used the name. Truncation handles suffix growth past nine collisions and trims an exposed trailing hyphen before appending the suffix. Regression tests enroll fifteen distinct keys with the same maximum-length name, check uniqueness and valid length, keep another network independent, preserve the original name across saved-credential rejoin/reload, and verify failed persistence leaves names/identity/IP allocation unchanged. Local race/build/vet checks pass; independent CI is pending. This is bounded enrollment name correctness, not a fifteen-device running VPN or capacity claim.

## Important open work

- Account/organization ownership, user login and device-management experience beyond the explicit operator membership gate.
- Wider subnet/firewall compatibility, non-Linux/IPv6/public subnet OS routes and owned exit routing.
- Real Windows Wintun/macOS VPN, second-user access, physical capture/input/permissions, mobile and actual OS boot/reboot.
- Larger/cancelled transfers, long sessions, sleep/wake, sustained churn and fleet/relay capacity.
- Durable hosted storage, recovery practice, independent security review and store-policy/data-disclosure review. Store copy remains a draft.

Methods and earlier failures: [native validation](native-platform-validation.md), [reliability](session-reliability.md), [operations](operations.md), [release history](RELEASES.md) and [synthetic scale measurements](scale-testing.md).
