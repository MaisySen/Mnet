# Mnet project status

Updated 2026-11-16, Australia/Sydney. Development/testing only. No production-readiness, parity, security-certification or supported fleet-size claim.

## Source, downloads and deployment

| Layer | Recorded state | Evidence / limit |
| --- | --- | --- |
| Published test artifacts | v0.8.2-test.1, source `2bb55f4be60969a855b37fbb6d607f0831542e45`, Go 1.26.11; Android 0.8.2/code 14. | [Release](https://github.com/MaisySen/Mnet/releases/tag/v0.8.2-test.1). Sixteen assets verified at release. Update CLI and daemon together. |
| Hosted test control plane | Last verified 2026-11-16 at 07:12 AEDT on `2bb55f4`. | Dated deployment check, not a current availability promise. Free-host ephemeral disk/restart state loss remains a limitation. |
| Newer verified source | `841d3a6`, all five jobs passed. | [Exact CI](https://github.com/MaisySen/Mnet/actions/runs/43795711303). New source below is not in those downloads or that last verified deployment. |

## What is implemented and tested

### Mesh, routing and recovery

- Three independently keyed Linux kernel VPN devices in one network: unique IPs/keys, two peers per device, six directed mesh pings and third-device OS TCP 3 x 4MiB with receiver hash/sender ACK. Removing that device preserves the original mesh.
- A fresh device in a second network on the same control service sees no original peers, is absent from original maps, and cannot reach distinct original mesh IPs. Network address spaces can overlap; its own IP is excluded from this test.
- Bounded authenticated direct probes and filtering of mesh/advertised-prefix underlay candidates address reproduced convergence/recursive-routing hazards. Direct paths are asserted, not inferred from healthy relay traffic.
- Managed Linux RFC1918 IPv4 subnet routes and destination-scoped NAT have explicit ownership, rollback and cleanup. The separate LAN host has no mesh return route. Serving requires operator-enabled forwarding; Mnet never changes that shared sysctl. Unrelated NAT/routes and the operator LAN route are preserved. Older broad NAT rules are not adopted or deleted.
- With forwarding disabled, serving setup ends Down without a TUN/NAT leak or unrelated changes. Retry succeeds after the fixture enables forwarding. Saved Down/Up cycles and a fresh daemon process using saved state and `-up` restore identity, direct VPN and LAN access.
- Route-only ACL withdrawal/restoration and admin expiry/reapproval block and restore access as expected. Reapproval clears stale approval status/link. Known expiry is enforced locally even if later control polling is blocked; unseen offline admin changes cannot be discovered.
- `mnet up --reset-routing`, while down, clears saved roles without deleting identity. Ordinary saved-state Up preserves routing. Fresh attempts clear old setup errors, preserving errors raised during the new attempt.

The latest verified [841d3a6 CI](https://github.com/MaisySen/Mnet/actions/runs/43795711303) repeated the full kernel fixture three times. Later original-pair/LAN transfers ran with control and relay stopped: LAN TCP 3 x 4MiB, mesh normal TCP 3 x 4MiB and mesh TCP with 1% loss/20ms delay 3 x 4MiB, with receiver hash/sender ACK. Actual loss drops: 146/140/144. Third-device transfers occurred before control was stopped. These are disposable controlled fixtures, not fleet or public-internet soak proof.

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

## Mesh address allocator bounds (verified source)

New device allocation stays inside usable `100.64.0.0/10` host addresses, excludes the prefix network/broadcast address and wraps a stale, negative or excessive saved cursor without moving existing devices. It skips addresses already used in that network and fails if the bounded pool is exhausted rather than overflowing outside the mesh prefix. Tests cover extreme cursors, a small isolated chooser's full-pool behavior, wrap/collision skipping, persistence/reload and exact failed-write rollback. Small-pool exhaustion tests do not prove millions of enrolled or connected devices. All five jobs passed in [3bc3dca CI](https://github.com/MaisySen/Mnet/actions/runs/43792404527), including the real-Chrome fixture and three complete kernel fixtures with 173/132/154 later loss-profile drops. Existing saved addresses are not migrated or repaired by this change.

## Fail-closed persisted-state shape (verified source)

An existing empty/non-object state file, explicit null required map or null network/node/enrollment-key record now refuses startup with a generic diagnostic instead of behaving like empty state or reaching a later panic. Rejection does not rewrite or delete the original file. Older object-format files with omitted fields still use initialized defaults; tests confirm network creation, enrollment, saved credentials and reload work without inferring identity. This is a narrow shape check, not full semantic validation, corruption repair or durable-host recovery. Tests verify rejection, unchanged file bytes and no record identifiers in the diagnostic. All five jobs passed in [c129350 CI](https://github.com/MaisySen/Mnet/actions/runs/43793287906), including three complete kernel fixtures with 136/146/121 later loss-profile drops. Missing files still start a new empty store, so operators must distinguish a missing ephemeral volume from a verified recovery.

## Private control-state file boundary (verified source)

Control startup now rejects a symlink/non-regular state file before reading it and restricts an existing regular file through the platform-private-file helper. Saves reject a linked/non-regular destination and apply that helper to the unpredictable temporary file before writing secret bytes. Unlike the daemon's dedicated-directory workflow, control does not tighten an arbitrary shared parent folder. Local tests cover existing permissive files, unchanged shared-parent modes, linked read/write refusal, untouched outside targets and failed-write state rollback. All five jobs passed in [da2ab34 CI](https://github.com/MaisySen/Mnet/actions/runs/43794111765), including native Windows/macOS private-file/shape regressions and three full kernel fixtures with 138/145/152 later loss-profile drops. This is not race-free defense against a malicious local administrator swapping path components; trusted private directory placement and OS access control remain necessary.

## Direct Windows control-state DACL assertion (verified native source)

A Windows-specific control regression starts with an Everyone-readable state file, opens it, checks that inheritance is disabled and grants are limited to its owner, LocalSystem and local administrators, then repeats the check after two actual atomic store replacements and reload. It also verifies the shared parent's security descriptor is unchanged and temporary files are gone. All five jobs passed in [bd9dcf7 CI](https://github.com/MaisySen/Mnet/actions/runs/43794863230), including execution of this assertion on native Windows and three complete kernel fixtures with 126/141/129 later loss-profile drops. This checks descriptors and owner access, not an actual second-user access attempt or malicious-administrator resistance.

## Reject broken saved device identities (verified source)

Daemon state loading rejects non-object files, malformed/half key pairs, mismatched private/public keys and saved control credentials without a key pair. EnsureKey also validates existing in-memory material before deciding whether first-run generation is allowed. It never silently replaces those broken identities. Valid saved pairs reload without regeneration/rewrite, while metadata-only legacy state can still generate its first pair. Tests check unchanged rejected file bytes, in-memory refusal without writing, generated-pair reload and metadata compatibility. All five jobs passed in [841d3a6 CI](https://github.com/MaisySen/Mnet/actions/runs/43795711303), including native Windows/macOS identity assertions and three full kernel fixtures with 146/140/144 later loss-profile drops. This does not repair corruption, rotate an identity or prove physical reboot recovery.

## Daemon failed-save rollback (new source, pending CI)

If the first identity save fails, EnsureKey now removes the unpublished in-memory pair so a retry cannot mistake it for a durable identity and skip saving. A successful retry is reloaded and checked against the generated pair. Routing-setting save failure also restores the previous live exit/routes/selection exactly, leaving the saved file intact; a later valid change persists correctly. Local daemon race/build/vet checks pass and native Windows/macOS regressions are added; independent CI is pending. These changes do not enable exit routing or repair/migrate existing identities.

## Important open work

- Account/organization ownership, user login and device-management experience beyond the explicit operator membership gate.
- Wider subnet/firewall compatibility, non-Linux/IPv6/public subnet OS routes and owned exit routing.
- Real Windows Wintun/macOS VPN, second-user access, physical capture/input/permissions, mobile and actual OS boot/reboot.
- Larger/cancelled transfers, long sessions, sleep/wake, sustained churn and fleet/relay capacity.
- Durable hosted storage, recovery practice, independent security review and store-policy/data-disclosure review. Store copy remains a draft.

Methods and earlier failures: [native validation](native-platform-validation.md), [reliability](session-reliability.md), [operations](operations.md), [release history](RELEASES.md) and [synthetic scale measurements](scale-testing.md).
