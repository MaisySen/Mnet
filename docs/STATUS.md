# Mnet project status

Updated 2026-11-16, Australia/Sydney. Development/testing only. No production-readiness, parity, security-certification or supported fleet-size claim.

## Source, downloads and deployment

| Layer | Recorded state | Evidence / limit |
| --- | --- | --- |
| Published test artifacts | v0.8.2-test.1, source `2bb55f4be60969a855b37fbb6d607f0831542e45`, Go 1.26.11; Android 0.8.2/code 14. | [Release](https://github.com/MaisySen/Mnet/releases/tag/v0.8.2-test.1). Sixteen assets verified at release. Update CLI and daemon together. |
| Hosted test control plane | Last verified 2026-11-16 at 07:12 AEDT on `2bb55f4`. | Dated deployment check, not a current availability promise. Free-host ephemeral disk/restart state loss remains a limitation. |
| Newer verified source | `086e99c`, all five jobs passed. | [Exact CI](https://github.com/MaisySen/Mnet/actions/runs/43797515391). New source below is not in those downloads or that last verified deployment. |

## What is implemented and tested

### Mesh, routing and recovery

- Three independently keyed Linux kernel VPN devices in one network: unique IPs/keys, two peers per device, six directed mesh pings and third-device OS TCP 3 x 4MiB with receiver hash/sender ACK. Removing that device preserves the original mesh.
- A fresh device in a second network on the same control service sees no original peers, is absent from original maps, and cannot reach distinct original mesh IPs. Network address spaces can overlap; its own IP is excluded from this test.
- Bounded authenticated direct probes and filtering of mesh/advertised-prefix underlay candidates address reproduced convergence/recursive-routing hazards. Direct paths are asserted, not inferred from healthy relay traffic.
- Managed Linux RFC1918 IPv4 subnet routes and destination-scoped NAT have explicit ownership, rollback and cleanup. The separate LAN host has no mesh return route. Serving requires operator-enabled forwarding; Mnet never changes that shared sysctl. Unrelated NAT/routes and the operator LAN route are preserved. Older broad NAT rules are not adopted or deleted.
- With forwarding disabled, serving setup ends Down without a TUN/NAT leak or unrelated changes. Retry succeeds after the fixture enables forwarding. Saved Down/Up cycles and a fresh daemon process using saved state and `-up` restore identity, direct VPN and LAN access.
- Route-only ACL withdrawal/restoration and admin expiry/reapproval block and restore access as expected. Reapproval clears stale approval status/link. Known expiry is enforced locally even if later control polling is blocked; unseen offline admin changes cannot be discovered.
- `mnet up --reset-routing`, while down, clears saved roles without deleting identity. Ordinary saved-state Up preserves routing. Fresh attempts clear old setup errors, preserving errors raised during the new attempt.

The latest verified [086e99c CI](https://github.com/MaisySen/Mnet/actions/runs/43797515391) repeated the full kernel fixture three times. Later original-pair/LAN transfers ran with control and relay stopped: LAN TCP 3 x 4MiB, mesh normal TCP 3 x 4MiB and mesh TCP with 1% loss/20ms delay 3 x 4MiB, with receiver hash/sender ACK. Actual loss drops: 140/123/128. Third-device transfers occurred before control was stopped. These are disposable controlled fixtures, not fleet or public-internet soak proof.

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


- Admin sessions are capped at 1,024 entries, reclaim expired entries, rotate an existing token on successful re-login and support server-side logout. Old tokens stop working. Sessions are process-local and disappear on restart; logout does not revoke device credentials or mesh traffic. [Session CI](https://github.com/MaisySen/Mnet/actions/runs/43787156366).

See [product scope](product-scope.md) for the account/device/startup target and platform gaps. The shared architecture blueprint is reference, not an adopted stack, budget or migration plan.

## Operator dashboard and enrollment correctness

- Sign out clears private state, tables, selectors and fields. A delayed pre-logout refresh cannot restore them. Sign-in and signed-in views are separate. The isolated real-Chrome regression runs against the actual disposable control service and checks login, logout, cookie removal and old-token rejection. [Browser CI](https://github.com/MaisySen/Mnet/actions/runs/43788090621).
- Device/key expiry shows positive remaining time, expired at/before the deadline, never or unknown. The key dialog no longer falsely says "shown once": stored enrollment keys remain visible to admins. The browser fixture creates a real one-hour key, checks formatter boundaries and signs out with the key table populated before releasing the held old refresh. Fixture credentials are not printed; preview captures redact them. [Populated browser CI](https://github.com/MaisySen/Mnet/actions/runs/43788935254).
- `online_by_node` records relay presence by node ID. Two identical keys in separate networks cannot falsely share presence or collapse the node count. The dashboard says "Relay connection", not whole-device online/offline. Direct traffic may work without relay. The legacy public-key `online` map remains for compatibility, not scoped presence. Real WebSocket tests connect/disconnect both nodes independently. [Presence CI](https://github.com/MaisySen/Mnet/actions/runs/43789815773).
- Duplicate device names keep a unique suffix inside the validated 32-character bound, including double-digit collisions. Tests preserve network scope and saved-identity names across reload/rejoin, and verify failed-save rollback. Fifteen synthetic enrollments test naming only, not fifteen active VPN devices. [Name CI](https://github.com/MaisySen/Mnet/actions/runs/43790701519).

The browser evidence is desktop Chrome on disposable state/profile, not a live-user account or physical mobile proof. Detailed methods, limitations and earlier failures remain in the guides below.

## Saved state and startup safety

| Boundary | Verified behavior | Evidence and limits |
| --- | --- | --- |
| Mesh IP allocation | New addresses stay in usable `100.64.0.0/10`, wrap stale/extreme cursors and skip occupied addresses. Existing devices are not moved. | [Allocator CI](https://github.com/MaisySen/Mnet/actions/runs/43792404527). Small-pool exhaustion tests are not millions-device capacity proof. Existing invalid addresses are not repaired. |
| Control state shape | Existing empty/non-object files, explicit null required maps and null network/node/key records refuse startup without rewriting source bytes. | [Shape CI](https://github.com/MaisySen/Mnet/actions/runs/43793287906). Older object files with omitted fields work. Missing files still mean new empty state. Not full semantic validation or repair. |
| Control private files | Linked/non-regular read/write targets are refused; existing files and atomic temporary replacements are protected without changing shared-parent permissions. | [Private-file CI](https://github.com/MaisySen/Mnet/actions/runs/43794111765). Native Windows/macOS checks pass. Not race-free defense against a malicious local administrator swapping paths. |
| Windows control DACL | Starting from a broad file, load and two atomic replacements retain a protected owner/System/admin-only descriptor; shared-parent security descriptor stays unchanged. | [Native DACL CI](https://github.com/MaisySen/Mnet/actions/runs/43794863230). Descriptor/owner-access proof, not an actual second-user access test. |
| Device identity | Non-object, malformed/half/mismatched key pairs and credentials without keys are refused without silent regeneration. Valid pairs reload without rewrite; metadata-only state can generate its first pair. | [Identity CI](https://github.com/MaisySen/Mnet/actions/runs/43795711303). Native Windows/macOS checks, not corruption repair or physical reboot proof. |
| Save failure | First-key save failure clears the unpublished pair, so retry actually saves. Routing changes restore prior live settings on save failure. Up/Prepare validation/config-save failure restores the prior server/network/name while keeping a durable identity. | [Identity/routing rollback CI](https://github.com/MaisySen/Mnet/actions/runs/43796613946), [startup rollback CI](https://github.com/MaisySen/Mnet/actions/runs/43797515391). Remote enrollment effects are outside this rollback boundary. |

These checks do not make ephemeral hosting durable. Keep state in a trusted private path, preserve recovery copies and distinguish a missing volume from a verified new installation. No storage migration or hosted account changes were made for this work.

## Control credential destination follow-up

New source refuses all daemon HTTP control API redirects, including same-origin redirects and supplied HTTP clients. A local two-server race regression covers all five redirect codes and proves the redirect destination receives no request; direct requests still work. Use the final base URL directly. Native/full CI verification is pending. This is not a live incident report or a deployment change. See [operations](operations.md#control-api-destination).

New source also rejects server/network changes for an enrolled saved credential in both Up and mobile Prepare. Local race tests verify rejection before requests or saved-state changes and a retry reaching the original server. Use the saved target or a separate state directory for a new identity. Network selectors compare exactly; this is not account switching or remote migration. Full/native CI is pending. See [target handling](operations.md#enrolled-target-changes).

## Important open work

- Account/organization ownership, user login and device-management experience beyond the explicit operator membership gate.
- Wider subnet/firewall compatibility, non-Linux/IPv6/public subnet OS routes and owned exit routing.
- Real Windows Wintun/macOS VPN, second-user access, physical capture/input/permissions, mobile and actual OS boot/reboot.
- Larger/cancelled transfers, long sessions, sleep/wake, sustained churn and fleet/relay capacity.
- Durable hosted storage, recovery practice, independent security review and store-policy/data-disclosure review. Store copy remains a draft.

Methods and earlier failures: [native validation](native-platform-validation.md), [reliability](session-reliability.md), [operations](operations.md), [release history](RELEASES.md) and [synthetic scale measurements](scale-testing.md).
