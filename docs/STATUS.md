# Mnet project status

Updated 2026-11-16, Australia/Sydney. Development/testing only. No production-readiness, parity, security-certification or supported fleet-size claim.

## Source, downloads and deployment

| Layer | Recorded state | Evidence / limit |
| --- | --- | --- |
| Published test artifacts | v0.8.2-test.1, source `2bb55f4be60969a855b37fbb6d607f0831542e45`, Go 1.26.11; Android 0.8.2/code 14. | [Release](https://github.com/MaisySen/Mnet/releases/tag/v0.8.2-test.1). Sixteen assets verified at release. Update CLI and daemon together. |
| Hosted test control plane | Last verified 2026-11-16 at 07:12 AEDT on `2bb55f4`. | Dated deployment check, not a current availability promise. Free-host ephemeral disk/restart state loss remains a limitation. |
| Newer verified source | `5e6e406`, all five jobs passed, including the complete operator-to-member browser journey. | [Exact CI](https://github.com/MaisySen/Mnet/actions/runs/43844911937). New source below is not in those downloads or that last verified deployment. |

## What is implemented and tested

### Mesh, routing and recovery

- Three independently keyed Linux kernel VPN devices in one network: unique IPs/keys, two peers per device, six directed mesh pings and third-device OS TCP 3 x 4MiB with receiver hash/sender ACK. Removing that device preserves the original mesh.
- A fresh device in a second network on the same control service sees no original peers, is absent from original maps, and cannot reach distinct original mesh IPs. Network address spaces can overlap; its own IP is excluded from this test.
- Bounded authenticated direct probes and filtering of mesh/advertised-prefix underlay candidates address reproduced convergence/recursive-routing hazards. Direct paths are asserted, not inferred from healthy relay traffic.
- Managed Linux RFC1918 IPv4 subnet routes and destination-scoped NAT have explicit ownership, rollback and cleanup. The separate LAN host has no mesh return route. Serving requires operator-enabled forwarding; Mnet never changes that shared sysctl. Unrelated NAT/routes and the operator LAN route are preserved. Older broad NAT rules are not adopted or deleted.
- With forwarding disabled, serving setup ends Down without a TUN/NAT leak or unrelated changes. Retry succeeds after the fixture enables forwarding. Saved Down/Up cycles and a fresh daemon process using saved state and `-up` restore identity, direct VPN and LAN access.
- Route-only ACL withdrawal/restoration and admin expiry/reapproval block and restore access as expected. Reapproval clears stale approval status/link. Known expiry is enforced locally even if later control polling is blocked; unseen offline admin changes cannot be discovered.
- `mnet up --reset-routing`, while down, clears saved roles without deleting identity. Ordinary saved-state Up preserves routing. Fresh attempts clear old setup errors, preserving errors raised during the new attempt.

The latest verified [5e6e406 CI](https://github.com/MaisySen/Mnet/actions/runs/43844911937) repeated the full kernel fixture three times. Later original-pair/LAN transfers ran with control and relay stopped: LAN TCP 3 x 4MiB, mesh normal TCP 3 x 4MiB and mesh TCP with 1% loss/20ms delay 3 x 4MiB, with receiver hash/sender ACK. Actual loss drops: 149/131/174. Third-device transfers occurred before control was stopped. These are disposable controlled fixtures, not fleet or public-internet soak proof.

Exit-node selection/advertising is explicitly rejected before enrollment/device creation. Unsafe dormant host/default-route replacement helpers were removed, not enabled. Exit internet routing remains unsupported.

### Platforms and sessions

- Linux startup setup (new source, CI pending): read-only `mnetd -service print-unit` emits a saved-state systemd unit with private umask/restart-on-failure. Actual CLI no-state-write checks and systemd syntax verification pass locally, along with full race/vet/cross-builds. Unsupported/secret overrides reject. No service installation, enablement or boot/reboot proof.

- Windows: native protected-DACL/private-state, local API, expiry, preparation/cancellation and loopback OS TCP regressions. Actual SCM install/start/stop/restart, forced-crash recovery and removal pass on a disposable runner. No Wintun VPN or physical reboot claim.
- macOS: native build/private-state/local-API, preparation/cancellation, expiry and loopback sessions. Actual VPN, physical capture/input, consent and launchd boot remain unverified.
- Shared mobile lifecycle: Prepare reports preparing, serializes with Up/Down, publishes identity under a lock and is cancellable by Down. State snapshots do not share mutable route slices. Local race regressions and native CI pass, not physical mobile proof.
- Android: signed test artifacts and a prior virtual Firebase Test Lab join. Physical VPN/capture/input/reconnect and reboot startup remain open. iOS has source/shared Go coverage, not signed TestFlight or physical-device proof.
- Sessions: isolated X11 changed-frame/chat/hash/ACK and bounded stress/loss proofs. Production sessions use OS TCP; the attributed gVisor adapter is proof-only.

## Account and operator access

Same-network multi-device connectivity is verified. A single login owning/managing all devices is not finished.

- Verified OIDC issuer/subject is stored as private approval provenance, separately from display email. Historical email-only labels remain unknown; principals are not added to peer maps. An internal own-device query returns only approved, unexpired summaries for an exact network + issuer + subject. Explicit membership is required by its gated entry point; approval alone never grants membership. The separate authenticated account query/login is described below; owner/admin roles remain unfinished. [Provenance CI](https://github.com/MaisySen/Mnet/actions/runs/43780929040), [signed callback CI](https://github.com/MaisySen/Mnet/actions/runs/43781793743), [query CI](https://github.com/MaisySen/Mnet/actions/runs/43782589045), [membership CI](https://github.com/MaisySen/Mnet/actions/runs/43783449695).
- `POST /admin/api/memberships` lets an authenticated operator explicitly grant/revoke an exact principal. A strict 4KiB body requires network/issuer/subject and an explicit boolean decision. The target principal is not caller authentication and must not be guessed from email. Persistence rollback and snapshot/audit non-disclosure are tested. Revocation closes this account-query gate only, not device credentials or mesh traffic. [Endpoint CI](https://github.com/MaisySen/Mnet/actions/runs/43784350674).
- Login and every admin mutation require JSON plus configured same-origin browser checks. State/metrics and membership inspection use GET, ACL uses GET/PUT, mutations use POST only. Headerless non-browser JSON scripts remain supported. HTTPS admin cookies are Secure, HttpOnly and SameSite=Lax. [Admin request guard CI](https://github.com/MaisySen/Mnet/actions/runs/43785399011).
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

## Control credentials and enrollment

| Boundary | Verified behavior | Evidence and limits |
| --- | --- | --- |
| Redirects | All control HTTP redirects fail before forwarding, including same-origin redirects and supplied clients. | [Redirect CI](https://github.com/MaisySen/Mnet/actions/runs/43799181639). Set the final base URL directly. |
| Enrolled target | Up/Prepare reject server/network changes before requests or saves. The original target remains usable. | [Target CI](https://github.com/MaisySen/Mnet/actions/runs/43799335015). Exact network selector; separate state directory for another identity, not account switching. |
| Enrollment response | Incomplete credentials cannot replace saved identity. Accepted credentials survive an in-process failed-save retry; no new client before persistence. | [Recovery CI](https://github.com/MaisySen/Mnet/actions/runs/43800305333). Does not undo remote enrollment or guarantee recovery after process exit. |
| Diagnostics | Printable transport errors and approval-wait logs omit credential URLs. Authenticated local status retains the approval action. | [Diagnostic CI](https://github.com/MaisySen/Mnet/actions/runs/43801230914). Historical/proxy logs are not scrubbed; no live exposure is established. |

Each run passed all five jobs, including native Windows/macOS and three full kernel fixtures. These are source changes, not deployment changes. See [destination setup](operations.md#control-api-destination), [target handling](operations.md#enrolled-target-changes), [enrollment recovery](operations.md#enrollment-response-and-local-save-recovery) and [diagnostics](operations.md#credential-safe-diagnostics) before changing an installed node.

### Response and authorization boundaries

| Boundary | Verified behavior | Evidence and limits |
| --- | --- | --- |
| Response size | Exactly 4 MiB is allowed; oversized responses fail explicitly, not by silent truncation. Body-read failures close the response and keep safe printed errors. | [Size CI](https://github.com/MaisySen/Mnet/actions/runs/43802190586). Existing bound, not a fleet-capacity claim. |
| Remote errors | Rejected requests print HTTP status, not credential-echoing error bodies. Unauthorized matching survives malformed/oversized replies. JSON causes remain typed behind safe printed text. | [Error CI](https://github.com/MaisySen/Mnet/actions/runs/43803217307). Private server/proxy logs remain separate. |
| Saved enrollment | Partial/blank credentials or missing server/network refuse startup without changing files or regenerating keys. Fresh unenrolled pairs remain allowed. | [State CI](https://github.com/MaisySen/Mnet/actions/runs/43804190569). Not corruption repair or remote token validation. |
| Received denial | Map/relay 403 or an unapproved map withdraws old peer access. Status waits for approval; path refresh cannot reinstall peers. A later approved map restores access. | [IPC CI](https://github.com/MaisySen/Mnet/actions/runs/43805223887), [kernel CI](https://github.com/MaisySen/Mnet/actions/runs/43806151062). Cannot infer unseen offline revocation. |

The denial extension passed three fresh complete Linux kernel fixtures: a stopped disposable control file marks only test node b unapproved; after restart its actual mesh/LAN traffic is denied until authenticated reapproval. Direct access returns with unchanged saved identities before the existing control-off TCP/loss/cleanup tests continue. This is test-only state injection, not a production unapprove API or live account change. See [operations](operations.md) and [native/kernel methods](native-platform-validation.md) for recovery instructions, methods and limits.

## CLI and member workflow

| Capability | Verified behavior | Evidence and limits |
| --- | --- | --- |
| Readiness wait | `mnet wait --timeout 90s` observes current authenticated local Up with an address. Timeout leaves daemon/identity unchanged, including while approval is pending. | [CLI CI](https://github.com/MaisySen/Mnet/actions/runs/43808113962), [kernel timeout CI](https://github.com/MaisySen/Mnet/actions/runs/43809073253). Not a particular Up request's completion or peer reachability. |
| Private JSON status | `mnet status --json` emits one authenticated local status object without human text. | [JSON CI](https://github.com/MaisySen/Mnet/actions/runs/43810178477). Approval URLs can contain credentials; do not publish this output. |
| Account session | Separate provider/principal/network sessions recheck explicit membership for each own-device read. Expiry/logout/capacity are bounded. | [Session CI](https://github.com/MaisySen/Mnet/actions/runs/43811262859). Never an operator/device credential or automatic membership. |
| Account HTTP | Browser-bound, one-use OIDC state issues a private account cookie only after explicit membership. Queries cannot select another principal/network; logout revokes the server token. | [HTTP CI](https://github.com/MaisySen/Mnet/actions/runs/43812377082). Additional provider callback registration is needed before live use; none was changed here. |
| Member page | `/account/` signs in, lists only the principal's approved/unexpired devices and signs out. Delayed pre-logout responses cannot repaint private rows. | [Browser CI](https://github.com/MaisySen/Mnet/actions/runs/43813517063), [edge-state CI](https://github.com/MaisySen/Mnet/actions/runs/43814532649). Real Chrome proof; edge responses are browser-local fixtures, 390px layout is not physical mobile proof. |

### Member and operator boundaries

| Capability | Verified behavior | Evidence and limits |
| --- | --- | --- |
| Provider change | Account sessions and pending logins are permanently cleared on provider replacement/disable. Restoring the old provider cannot revive them. | [Revocation CI](https://github.com/MaisySen/Mnet/actions/runs/43815622508). No live provider changed. |
| Sign-in recovery | Generic failed-callback page returns to sign-in without echoing private parameters or claiming an older session ended. | [Recovery CI](https://github.com/MaisySen/Mnet/actions/runs/43816760633). |
| Two owned devices | Actual signed-provider API/DOM show exactly two owned approved/unexpired summaries; same-email/different-subject, other-issuer/network and expired records are excluded. | [Scope CI](https://github.com/MaisySen/Mnet/actions/runs/43817883961). Control records, not two new physical VPN devices. |
| Operator inspection | Private exact-network explicit grant listing is sorted/copied and no-store. Only an operator session can read it. Selection/sign-out clears rows and suppresses stale results. | [Inspection CI](https://github.com/MaisySen/Mnet/actions/runs/43819448319). No principal inference from email or approval. |
| Operator removal | Confirmation names an inspected exact network/issuer/subject and warns VPN connections stay separate. Cancel does not write. An uncertain outcome is settled by inspection before retry. | [Removal CI](https://github.com/MaisySen/Mnet/actions/runs/43823084397), [lost-response/stale-action CI](https://github.com/MaisySen/Mnet/actions/runs/43824315289). [Grant form CI](https://github.com/MaisySen/Mnet/actions/runs/43841705310) verifies manual principal entry, required acknowledgement, confirmation and refresh. No identity inference from email/approval. |
| Account/node separation | Rejected cross-origin removal preserves account access; accepted removal denies active account reads while the original approved node still authenticates its map. | [Signed HTTP/native CI](https://github.com/MaisySen/Mnet/actions/runs/43825558997). HTTP authorization proof, not kernel traffic proof. |
| Member entry and rendering | Landing links member and operator views separately. Complete response validation prevents malformed/partial device output; latest refresh wins. HTML-like names stay text, and empty/denied/390px states are checked. | [Entry CI](https://github.com/MaisySen/Mnet/actions/runs/43826840257), [response/browser CI](https://github.com/MaisySen/Mnet/actions/runs/43829486186). Edge responses are browser-local fixtures; initial OIDC/device reads are actual server-backed. Not physical mobile proof. |
| Network context | The member page shows only the authenticated session network ID; queries cannot change it and denial/sign-out clears it. | [Context CI](https://github.com/MaisySen/Mnet/actions/runs/43838497515). No issuer/subject/role fields added. |
| Logout uncertainty | Actual account logout can commit with its response dropped: rows stay cleared, no false confirmation, old-token replay denied, idempotent retry confirms sign-out. | [Logout CI](https://github.com/MaisySen/Mnet/actions/runs/43830783982). No live account changed. |

The member page states that approval is not live connectivity and sign-out does not disconnect VPN devices. Account login is read-only device-summary access, not enrollment, automatic membership, ownership/admin roles or device management. Live use needs additional provider callback registration; none was changed. See [operations](operations.md) for setup/private-data limits and [validation](native-platform-validation.md) for detailed methods, including the earlier Chrome startup failure and its bounded diagnostic follow-up.

## Important open work

- Account/organization ownership, enrollment and device management beyond the read-only member login and explicit operator membership gate.
- Wider subnet/firewall compatibility, non-Linux/IPv6/public subnet OS routes and owned exit routing.
- Real Windows Wintun/macOS VPN, second-user access, physical capture/input/permissions, mobile and actual OS boot/reboot.
- Larger/cancelled transfers, long sessions, sleep/wake, sustained churn and fleet/relay capacity.
- Durable hosted storage, recovery practice, independent security review and store-policy/data-disclosure review. Store copy remains a draft.

Methods and earlier failures: [native validation](native-platform-validation.md), [reliability](session-reliability.md), [operations](operations.md), [release history](RELEASES.md) and [synthetic scale measurements](scale-testing.md).

## Trace test isolation follow-up (verified)

The full race suite on [90f633b](https://github.com/MaisySen/Mnet/actions/runs/43820313703) detected a metadata-trace test mutating a global flag while a previous TCP test's writer still read it. The subsequent green run does not close that race. New source captures trace enablement per session before loops start and tests modify only their own unstarted fixture. Thirty local race repetitions of trace/TCP cases and a fresh full race suite/vet pass. All five jobs passed in [03b55a6 CI](https://github.com/MaisySen/Mnet/actions/runs/43821694030) and [b3f313d CI](https://github.com/MaisySen/Mnet/actions/runs/43821716633); the latter includes 30 Linux trace/TCP race repetitions, native three-repetition trace/TCP checks and both actual-Chrome flows. Tracing remains opt-in, metadata-only and bounded; this is not a new transport or live deployment.
