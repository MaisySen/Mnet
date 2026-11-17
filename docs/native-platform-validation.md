# Native platform validation

Native CI is a narrower claim than a physical desktop/phone test. Always name the layer that passed.

## Evidence routes

- [Platform matrix](#what-the-platform-matrix-checks) and [remaining platform work](#still-outstanding).
- [Three-device kernel mesh](#three-device-kernel-mesh), [network isolation](#cross-network-kernel-isolation) and [received-denial kernel recovery](#direct-kernel-received-denial-extension-verified-fixture).
- [CLI readiness](#cli-readiness-wait-verified-source), [approval timeout](#cli-approval-timeout-kernel-extension-verified-fixture) and [private JSON](#private-machine-readable-cli-status-verified-source).
- [Account HTTP](#account-http-plumbing-verified-source), [member page](#member-account-page-verified-browser-source) and [two-device summary scope](#two-device-account-summary-scope-verified-browser-extension).
- [Current result summary](STATUS.md) and [operating procedures](operations.md). The sections below preserve implementation-stage evidence; later extensions may replace an earlier limitation.

## What the platform matrix checks

- Windows and macOS build every package on a native runner, not only a cross-compiler.
- Local API lifecycle and capability tests exercise the runner's local transports.
- Private state tests exercise actual file replacement, link rejection and cleanup. Windows tests read the actual security descriptor, require a protected DACL, and reject grants outside the file owner, LocalSystem and local administrators. This is ACL inspection, not an impersonated second-user penetration test.
- Daemon tests exercise state round trips and WireGuard IPC using an in-memory TUN. Local-expiry regressions cover self/peer deadlines, renewal, stale-map refresh, and repeated Up/Down while later control polls are blocked. They do not create an OS VPN interface.
- Windows CI also installs a real SCM service in an offline fixture, checks authenticated local API startup, clean stop/restart, forced-process crash recovery and removal. The script refuses to replace an existing service. This does not create a Wintun interface or test VPN reconnection.
- Linux kernel CI creates independent real TUN interfaces in isolated network namespaces, checks assigned addresses and mesh ping, transfers 3 x 4MiB OS TCP payloads normally and 3 x 4MiB under 1% loss plus 20ms delay with sender ACK and receiver hash, asserts direct UDP endpoints, stops the control/relay process before transfers, requires actual nonzero drop counters, then checks interface and mesh-route removal. This is a controlled runner fixture, not physical hardware, the public internet, arbitrary-loss or long-session proof.
- Session tests exercise a native loopback TCP connection, screen/input/chat framing and a verified 3MiB file. They also test queued startup and repeated stream cleanup. They do not cross an actual VPN interface or a lossy public network.

Evidence for the combined direct-path check: [c4a1bcd, all five jobs](https://github.com/MaisySen/Mnet/actions/runs/43760903768). Newer source is distinct from published artifacts; see [STATUS](STATUS.md).

Bounded Linux RFC1918 subnet routing: [8fcaf91 CI](https://github.com/MaisySen/Mnet/actions/runs/43764310572) checks an advertised prefix reaches an address on the peer loopback, owned-route cleanup/reinstallation across two saved-state cycles, and preservation of an unrelated underlay route. That earlier revision did not forward through a separate LAN host; the later LAN extension below does. Exit routing, IPv6 and non-Linux subnet routing remain unverified.

Three fresh full fixture runs passed in [2d23b6a CI](https://github.com/MaisySen/Mnet/actions/runs/43765976816), including route-only ACL withdrawal/restoration with peer/unrelated-route preservation and actual drops 135/149/169. This is bounded regression repeatability, not a soak.

## Private state behavior

The daemon's dedicated state directory is restricted before secret bytes are created. On Unix it is 0700 and identity files are 0600. On Windows a protected DACL replaces inherited grants; the existing owner, LocalSystem and local administrators retain full access. Administrators remain trusted, as they can control the service itself.

Existing saved identity and capability files are restricted when the daemon opens them. Identity saves use a private unpredictable temporary file, sync, then replacement. A linked identity/token destination is rejected. The daemon may fail closed when it cannot set the required security descriptor. This does not clean up copies created elsewhere by older versions.

Use a dedicated state directory, not a shared document folder or a directory owned by another account. A custom shared path is not a way to share daemon-control permission.

## Still outstanding

- Separate non-admin Windows identity cannot read/change the saved state or use the local API.
- Real Windows Wintun interface/routing and service VPN reconnection behavior. Basic SCM lifecycle passes on a disposable native runner; physical deployment remains untested.
- Physical macOS/Windows capture, input consent, sleep/wake and network changes.
- Real GNOME portal/PipeWire capture and input behavior.
- Physical Android VPN, transfer, capture and reconnect checks.
- Broader OS TCP VPN testing: Windows/macOS interfaces, constrained bandwidth, larger files, cancellation and long sessions. The bounded Linux namespace transfer/loss profile above passes; this does not establish general reliability.
- Fleet/soak tests, persistent production hosting, independent security review and operational recovery.

No release or passing CI run alone closes this list.

## Separate LAN extension

New source extends the isolated kernel topology with a fourth namespace: a LAN host without a mesh return route. It asserts owned NAT cleanup on Down/Up, unrelated NAT and forwarding preservation, reapproval status, and control-off OS TCP to that LAN host. Local serving ownership/rollback and daemon race tests pass. All five jobs and three fresh full fixtures passed in [6735500 CI](https://github.com/MaisySen/Mnet/actions/runs/43770289687). Each also passed control-off LAN OS TCP 3 x 4MiB with receiver hash/sender ACK; later mesh loss profiles observed 154/120/114 drops. The fixture explicitly enables forwarding; Mnet does not change shared sysctls. A newer extension first disables forwarding and expects setup failure with no TUN/NAT leak, unchanged sysctl and unrelated NAT, then enables it and retries. This failure-path extension passed all five jobs and three fresh [fcbddd1 fixtures](https://github.com/MaisySen/Mnet/actions/runs/43771745649), including 147/154/110 later loss-profile drops.

## Routing reset and retry status

A later local regression reproduced stale last_error after a successful retry. New source clears errors when a fresh Up attempt starts, preserving errors raised during that attempt. The stronger kernel retry assertion and native regression passed all five jobs and three fresh [4127209 fixtures](https://github.com/MaisySen/Mnet/actions/runs/43772484156), including 140/186/158 later loss-profile drops.

New source adds `mnet up --reset-routing` (while down) to clear saved serving/exit roles without deleting identity. Ordinary saved-state Up keeps its previous routing configuration. Reset cannot be combined with new routing roles. A local API regression reproduced the missing recovery path before this change; local API tests pass, including 20 race-detector repetitions. A bounded 30-second startup reachability assertion and explicit saved-identity comparison passed all five jobs and three fresh [85c2f79 fixtures](https://github.com/MaisySen/Mnet/actions/runs/43774062502), including 123/157/150 observed later loss-profile drops. This is bounded startup convergence, not arbitrary network readiness.

A newer kernel extension gracefully stops the serving daemon process and starts a fresh process with saved state and `-up`, asserting unchanged identity and restored direct VPN/LAN. The first fresh-process restart passed, but the later one-shot LAN ping after ACL restoration failed in [b4046ed CI](https://github.com/MaisySen/Mnet/actions/runs/43774744281). Diagnostics also exposed a candidate inside an advertised subnet, which can route the tunnel endpoint back into itself. New source filters mesh/advertised-prefix endpoint candidates and uses bounded LAN convergence after route restoration; the kernel job passed three fresh full fixtures in [c33c6c5 CI](https://github.com/MaisySen/Mnet/actions/runs/43775496938), including fresh-process saved-state startup and 141/129/123 later loss-profile drops. The overall run was not green: the Go job failed one-second local API readiness and safe-file receive timeouts during the parallel race suite. Test-only readiness/receive budgets are now ten seconds; protocol/content/cleanup assertions and production timeouts are unchanged. All five jobs and three fresh full fixtures passed in [64eb696 CI](https://github.com/MaisySen/Mnet/actions/runs/43776218766), including 124/123/152 later loss-profile drops. The bounded process-startup batch is independently green. This is not a real reboot, systemd/launchd boot or forced-crash cleanup proof.

## Mobile preparation lifecycle (new source)

A fresh regression found that mobile Prepare still reported Down while enrollment was running, allowing other lifecycle operations to overlap. New source serializes Prepare with Up/Down, reports preparing, publishes initial and joined identity under the status lock, and returns independent routing slices from State/SetRouting. Prepare/API regressions passed 20 local race-detector repetitions and the full race suite passed locally. All five jobs passed in [718f74e CI](https://github.com/MaisySen/Mnet/actions/runs/43776983336), including native Windows/macOS preparation regressions and three full kernel fixtures with 152/152/152 later loss-profile drops. This is not physical mobile or automatic boot proof.

Preparation cancellation follow-up: the first serialization regression deliberately released a blocked join before Down could finish. A new regression showed Down could instead wait for the full preparation timeout. Prepare now publishes its cancellation function while entering preparing, so Down cancels blocked enrollment before joining the lifecycle lock. Blocked-Prepare cancellation and identity/status regressions passed 20 local race-detector repetitions; the full race suite/build/vet pass. All five jobs passed in [973a7e1 CI](https://github.com/MaisySen/Mnet/actions/runs/43777723099), including native cancellation regressions and three full kernel fixtures with 152/132/152 later loss-profile drops.

## Three-device kernel mesh

A new fixture adds a third independently keyed kernel VPN device to the same network, requires unique keys/IPs, two visible peers per device, all six directed mesh pings and third-device OS TCP 3 x 4MiB hash/ACK. It then removes that device and requires the original direct mesh to recover before the existing ACL/reapproval/control-off tests. All five jobs and three fresh fixtures passed in [799f3a1 CI](https://github.com/MaisySen/Mnet/actions/runs/43778473220), with 130/150/164 later loss-profile drops. Third-device transfers used an available control service; only later original-pair/LAN transfers were control-off. This does not implement a single-account ownership/login model or establish fleet capacity.

## Cross-network kernel isolation

A fresh identity joins a second network on the same disposable control service. The new fixture requires no original peers on that device, no outsider in the original pair maps, and failed access to distinct original mesh IPs. Independent network address spaces can overlap, so the isolated device's own IP is not treated as a cross-network target. The first run passed those assertions but failed a later reapproval helper in [b650b3e CI](https://github.com/MaisySen/Mnet/actions/runs/43779251604). The extended fixture repeatedly logged in instead of reusing its session; the observed failure is consistent with its login burst exceeding the existing limiter. The fixture now reuses its private temporary admin cookie rather than weakening the production limit. All five jobs and three complete fixtures then passed in [e0a8f84 CI](https://github.com/MaisySen/Mnet/actions/runs/43780009817), including the three-device mesh, cross-network isolation and later control-off profiles with 135/140/168 drops. Network-scoped transport isolation is not account ownership, organization membership or a multi-tenant security certification.

## OIDC approval provenance

Verified OIDC approval now stores issuer and subject separately from the display email in private control state. No principal is inferred for historical email-only approvals. Identity provenance persists atomically with approval, rolls back on save failure and is removed with the device. It is not added to peer-facing node maps. Local persistence, legacy-unknown, issuer-distinction, non-disclosure and failed-write regressions pass. This is account groundwork, not account ownership, membership, authorization based on email, a login dashboard or automatic device grouping. All five jobs passed in [c2fe64c CI](https://github.com/MaisySen/Mnet/actions/runs/43780929040), including three complete kernel fixtures with 131/144/138 later loss-profile drops. A further signed-provider callback regression now asserts the private issuer/subject was recorded and excluded from the returned map; the extra assertion passed all five jobs in [471f537 CI](https://github.com/MaisySen/Mnet/actions/runs/43781793743), with 149/143/176 later kernel loss-profile drops.

## Private identity-device query (verified source)

An internal helper can list approved, unexpired device summaries for an exact network + verified issuer + subject. It excludes same-email/different-subject identities, identical subjects at other issuers, other networks, legacy email-only approvals and expired devices. Returned summaries contain no keys/tokens/endpoints and do not alias stored state. Local race regressions cover persistence, scope, unknown inputs and expiry. All five jobs passed in [11198e8 CI](https://github.com/MaisySen/Mnet/actions/runs/43782589045), including three full kernel fixtures with 135/145/156 later loss-profile drops. No HTTP route exposes this helper; any future caller must authenticate the principal and separately authorize network access. This is not ownership, membership, a login UI or an authorization grant.

## Explicit network membership gate (verified source)

Private control-state membership now binds an exact network/issuer/subject tuple by an explicit internal operator/admin method. Verified approval alone never grants membership. A separate gated device query denies missing/revoked/wrong-network/wrong-issuer/wrong-subject membership and returns only that principal's approved, unexpired devices, not everyone in the network. Grant/revoke persist with rollback on save failure; tests cover restart, tuple collisions, unknown scope and no cross-principal disclosure. The verified membership model was internal-only; a newer admin-only management endpoint is described below. There is still no HTTP account-query route. Member is the only access shape; owner/admin roles and a user-facing login remain unfinished. Revoking this membership only closes the account-query gate, not device transport credentials; explicit device expiry is still required to revoke mesh traffic. All five jobs passed in [7e458a3 CI](https://github.com/MaisySen/Mnet/actions/runs/43783449695), including three full kernel fixtures with 139/143/140 later loss-profile drops.

## Operator membership management (verified source)

`POST /admin/api/memberships` accepts `network_id`, `issuer`, `subject` and an explicit boolean `allow`. It requires an unexpired admin session, JSON content type, a 4KiB bounded strict body and no trailing JSON. Browser requests must have the configured same origin when Origin is present and cannot claim cross-site or sibling-site fetch metadata. Non-browser operator clients may omit those browser headers. The principal in the body is the grant target, never the caller's authentication. There is no GET/list route, self-service membership, owner role, account login or device-query endpoint. The target principal must be selected from trustworthy identity evidence, not an email guess. Grant/revoke auditing records the network and outcome, not issuer/subject. A failed persistence attempt returns a generic error and rolls back the live membership. Local race tests cover those HTTP boundaries, expired/forged/absent sessions, persisted grants/revokes and snapshot/audit non-disclosure. All five jobs passed in [f42cb58 CI](https://github.com/MaisySen/Mnet/actions/runs/43784350674), including three full kernel fixtures with 167/136/134 later loss-profile drops. This endpoint is not yet in published downloads or the hosted deployment last verified above.

## Consistent admin request guard (verified source)

All existing admin mutations and login now require JSON and the same browser-origin checks used for membership management. Authenticated state/metrics reads accept GET, ACL accepts GET/PUT, and other management actions accept POST only; GET/HEAD/other methods cannot approve, delete, expire or change policy. Headerless non-browser JSON clients remain supported. HTTPS-configured servers now issue the admin cookie with Secure as well as HttpOnly and SameSite=Lax. Local race regressions exercise every registered admin endpoint/method combination, rejected cross-origin requests without state changes, login rejection without sessions, accepted same-origin/script writes and HTTP-versus-HTTPS cookie flags. All five jobs passed in [1614abd CI](https://github.com/MaisySen/Mnet/actions/runs/43785399011), including three complete kernel fixtures with 172/138/158 later loss-profile drops. This does not add user login, device management UI or transport revocation through membership.

## Bounded OIDC round trips (verified source)

Pending OIDC login state is capped at 1,024 entries. Expired states are reclaimed before capacity is checked; a full live table returns 429 with Retry-After rather than evicting active logins or growing without bound. Each state binds the provider instance that started it, so replacing the configured provider cannot finish an earlier provider's pending approval. Expiry equality is rejected. Start/callback support GET only and reject other methods before allocating/consuming state. Callback state remains one-use, including provider denials. Local race regressions cover capacity, reclamation, method restrictions, provider mismatch, unbound/expired states and replay; the existing signed-provider round trip still passes. All five jobs passed in [720e53d CI](https://github.com/MaisySen/Mnet/actions/runs/43786272564), including three complete kernel fixtures with 142/168/128 later loss-profile drops. This is bounded login-state handling, not browser-session account login or a whole-service denial-of-service guarantee.

## Bounded admin sessions and logout (verified source)

The in-memory admin session table is capped at 1,024 entries. Successful password checks reclaim expired entries before allocation; a full live table rejects a new session with 429 rather than evicting active users. Signing in with an existing valid session rotates that token without increasing the table size, and the old token stops working. `POST /admin/api/logout` requires the same session/JSON/origin guard as other mutations, deletes the server-side token and clears the protected cookie. Replaying the old cookie is unauthorized. Expired authorization lookups also remove the stale entry. Local race regressions cover capacity/reclamation, rotation at capacity, failed-password non-revocation, cross-site logout refusal, cookie clearing and old-token replay. All five jobs passed in [9460300 CI](https://github.com/MaisySen/Mnet/actions/runs/43787156366), including three complete kernel fixtures with 153/123/137 later loss-profile drops. Sessions remain process-local and disappear on restart; logout does not revoke device credentials or change mesh transport.

## Dashboard sign-out and real-browser regression (verified source)

The operator dashboard now has Sign out, clears rendered private state after logout and ignores a delayed pre-logout state/ACL response instead of reopening the dashboard. A CSS specificity bug that left the sign-in form visible while signed in is fixed. An isolated real-Chrome fixture runs the actual Go control service with disposable state/profile: signs in, sees its fixture network, signs out, releases an intentionally delayed valid refresh and checks the private UI stays cleared. It also verifies cookie removal and 401 when the old token is replayed. The local fixture passed and signed-in/signed-out screenshots were visually inspected. All five jobs passed in [0c61bcc CI](https://github.com/MaisySen/Mnet/actions/runs/43788090621), including the real-Chrome fixture, corrected unsupported-exit help and three full kernel fixtures with 166/146/156 later loss-profile drops. It uses no live account, user profile or hosted control state and is desktop Chrome proof only.

## Dashboard expiry truth and populated cleanup (verified source)

Key and device expiry display now distinguishes positive time remaining, expired at/before the deadline, no expiry and an invalid timestamp. It no longer converts a future timestamp into a negative "seconds left" value. The new-key dialog no longer claims the credential is shown only once while the admin table still exposes it. The real-Chrome fixture now creates an actual one-hour enrollment key through the dashboard, verifies the populated expiry row and checks exact formatter boundaries. It then signs out with that private row populated and releases the held old state response, asserting the key table and network selectors stay empty. No fixture credentials are printed or captured in screenshots. All five jobs passed in [10c8c1c CI](https://github.com/MaisySen/Mnet/actions/runs/43788935254), including the populated real-Chrome fixture and three full kernel fixtures with 155/158/132 later loss-profile drops. Admin access to stored keys remains an explicit trust boundary, not a one-time-secret design.

## Node-scoped relay presence (verified source)

The admin state now adds `online_by_node`, keyed by node ID rather than public key. The dashboard uses this map and calls the column "Relay connection", with "connected" / "not connected" labels. A missing relay connection is not proof that a direct-connected device is offline. Identical keys in different networks can no longer make another node appear connected or collapse the current relay node count. The legacy public-key `online` map remains for compatibility; it is not a network-scoped presence source. Real WebSocket regressions connect two same-key nodes in different networks, inspect authenticated state/count, disconnect them independently and check snapshot copy safety. All five jobs passed in [a08a67c CI](https://github.com/MaisySen/Mnet/actions/runs/43789815773), including the real-Chrome fixture and three full kernel fixtures with 138/134/139 later loss-profile drops.

## Bounded multi-device collision names (verified source)

Repeated device names now keep the automatic numeric suffix inside the same 32-character validated name bound. A maximum-length request previously grew beyond that bound when another device already used the name. Truncation handles suffix growth past nine collisions and trims an exposed trailing hyphen before appending the suffix. Regression tests enroll fifteen distinct keys with the same maximum-length name, check uniqueness and valid length, keep another network independent, preserve the original name across saved-credential rejoin/reload, and verify failed persistence leaves names/identity/IP allocation unchanged. All five jobs passed in [38bb26f CI](https://github.com/MaisySen/Mnet/actions/runs/43790701519), including the real-Chrome fixture and three complete kernel fixtures with 116/127/145 later loss-profile drops. This is bounded enrollment name correctness, not a fifteen-device running VPN or capacity claim.

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

## Daemon failed-save rollback (verified source)

If the first identity save fails, EnsureKey now removes the unpublished in-memory pair so a retry cannot mistake it for a durable identity and skip saving. A successful retry is reloaded and checked against the generated pair. Routing-setting save failure also restores the previous live exit/routes/selection exactly, leaving the saved file intact; a later valid change persists correctly. All five jobs passed in [ba15a1d CI](https://github.com/MaisySen/Mnet/actions/runs/43796613946), including native Windows/macOS rollback regressions and three full kernel fixtures with 162/155/150 later loss-profile drops. These changes do not enable exit routing or repair/migrate existing identities.

## Startup configuration save rollback (verified source)

Up and mobile Prepare now restore the prior server/network/device-name settings if initial validation or saving the replacement configuration fails. A failed call cannot leave a different unsaved destination in memory for a later retry. A successfully saved first key pair is kept; this change does not discard a durable identity. Local race tests exercise both entry points with an existing pair and a failing destination, checking prior live configuration, unchanged disk bytes/keys and Down status. All five jobs passed in [086e99c CI](https://github.com/MaisySen/Mnet/actions/runs/43797515391), including native Windows/macOS startup rollback regressions and three full kernel fixtures with 140/123/128 later loss-profile drops. Enrollment that has already changed remote control state is outside this narrow rollback boundary.

## Control credential destination (verified source)

The HTTP control client refuses 301/302/303/307/308 redirects before forwarding a request to another destination, including same-origin redirects and supplied clients. Up and mobile Prepare also reject changes to an enrolled identity's saved server/network before any request or configuration save. The redirect run passed all five jobs in [9d6c03d CI](https://github.com/MaisySen/Mnet/actions/runs/43799181639), including native Windows/macOS and three full kernel fixtures with 141/144/167 later loss-profile drops. Target-binding regressions passed all five jobs in [c632d45 CI](https://github.com/MaisySen/Mnet/actions/runs/43799335015), including native Windows/macOS and three full kernel fixtures with 151/143/143 drops. Configure the final base URL directly; existing network selectors compare exactly. These are isolated test proofs, not a live attack report, account-switching UI or migration/remote removal.

## Enrollment-response and save-retry boundary (verified source)

Missing/blank join credentials are rejected before replacing local identity. A valid accepted remote enrollment whose local save fails keeps its credential for an in-process retry while the old disk state stays unchanged; no new control client is published until saving succeeds. A retry test checks the accepted credential is sent and the same key pair reloads. All five jobs passed in [c8ff4bb CI](https://github.com/MaisySen/Mnet/actions/runs/43800305333), including native Windows/macOS and three full kernel fixtures with 152/145/136 later loss-profile drops. This is same-process recovery, not a crash-durable transaction. Remote node creation/one-time-key use is not undone, and process exit before saving can lose the accepted credential.

## Credential-safe printed diagnostics (verified source)

Verbose transport failures and approval-wait logs no longer print credential-bearing URLs. Tests retain error cause/cancellation matching and the authenticated local approval URL while checking no token is logged. All five jobs passed in [c9f1197 CI](https://github.com/MaisySen/Mnet/actions/runs/43801230914), including native Windows/macOS and three full kernel fixtures with 154/142/140 later loss-profile drops. No historical/proxy log scrubbing or live incident claim.

## Explicit response-body bound (verified source)

Control replies beyond the existing 4 MiB limit are explicitly rejected rather than silently truncated. Tests accept an exactly limit-sized valid reply, reject oversized replies and check safe failing-reader diagnostics/cause matching/body close. All five jobs passed in [8fb969a CI](https://github.com/MaisySen/Mnet/actions/runs/43802190586), including native Windows/macOS and three full kernel fixtures with 131/149/132 later loss-profile drops. Not a fleet-capacity claim.

## Remote error boundary (verified source)

Rejected control requests report HTTP status without arbitrary remote error bodies. Unauthorized matching is decided before reading, so malformed/oversized 401 bodies cannot hide the rejoin signal. Typed JSON decode causes remain available behind safe printable text. All five jobs passed in [4821002 CI](https://github.com/MaisySen/Mnet/actions/runs/43803217307), including native Windows/macOS and three full kernel fixtures with 137/119/140 later loss-profile drops. Private server/proxy logs remain separate.

## Incomplete saved enrollment (verified source)

Partial/blank node credentials or enrolled state without a server/network refuse startup without rewriting state or regenerating keys. Complete enrollment reloads and fresh unenrolled pairs remain allowed. All five jobs passed in [929b744 CI](https://github.com/MaisySen/Mnet/actions/runs/43804190569), including native Windows/macOS and three full kernel fixtures with 147/149/143 later loss-profile drops. No repair or remote token validity claim.

## Received denial withdrawal (verified IPC source)

All five jobs passed in [0aaf471 CI](https://github.com/MaisySen/Mnet/actions/runs/43805223887), including native Windows/macOS actual in-memory WireGuard withdrawal/path-refresh/recovery tests and three full kernel regression fixtures with 135/128/125 later loss-profile drops. The full fixture did not yet directly inject the new denial case; do not count it as kernel denial proof. Offline unseen revocation remains unknowable.

## Direct kernel received-denial extension (verified fixture)

All five jobs passed in [efaf16e CI](https://github.com/MaisySen/Mnet/actions/runs/43806151062). Three fresh disposable Linux fixtures stopped their control process, marked only test node b unapproved in that stopped private state file and restarted the same endpoint. Each required b waiting for approval with no peers, failed actual b-to-a mesh and a-to-LAN pings, authenticated reapproval, restored direct/LAN access and unchanged saved identities. Each then completed the existing control-off transfer/loss/cleanup suite with 141/155/131 later loss-profile drops. This is direct kernel denial/recovery proof for this bounded fixture, not a production unapprove endpoint, live account edit, offline-unseen-revocation guarantee or physical-device proof.

## CLI readiness wait (verified source)

All five jobs passed in [7b6e9ad CI](https://github.com/MaisySen/Mnet/actions/runs/43808113962), including native Windows/macOS CLI tests and three full kernel fixtures. Each ran the actual CLI against the authenticated Unix local API, required nonzero wait after intentional forwarding-disabled startup failure, and required success after Up with an address. Each then passed the kernel denial/reapproval and control-off transfer/loss/cleanup tests with 135/138/140 later loss-profile drops. This is current-state readiness, not per-request completion, peer reachability or physical boot proof.

## CLI approval-timeout kernel extension (verified fixture)

All five jobs passed in [e15f818 CI](https://github.com/MaisySen/Mnet/actions/runs/43809073253). In three fresh full kernel fixtures the actual CLI wait timed out during denied authorization, left waiting-approval and saved identity unchanged, then succeeded after reapproval. Each continued through control-off TCP/loss/cleanup with 131/143/123 later loss-profile drops. Timeout is not cancellation or rollback of the daemon.

## Private machine-readable CLI status (verified source)

All five jobs passed in [fcf0605 CI](https://github.com/MaisySen/Mnet/actions/runs/43810178477), including native Windows/macOS CLI tests and three full kernel fixture checks parsing the actual CLI JSON status and requiring Up/address without saved secret fields. The full suite continued through denial/CLI timeout/reapproval/control-off TCP/loss/cleanup with 153/160/149 later loss-profile drops. Pending approval URLs remain private token-bearing data; JSON output is not share-safe.

## Account HTTP plumbing (verified source)

All five jobs passed in [c86ac3b CI](https://github.com/MaisySen/Mnet/actions/runs/43812377082), including native Windows/macOS signed-provider account HTTP/browser-binding/session tests. The full kernel suite passed with 155/128/145 later loss-profile drops, as VPN regression coverage, not account UI proof. The newer member page has separate verified real Chrome evidence below. No live provider or deployment was changed.

## Member account page (verified browser source)

All five jobs passed in [031be71 CI](https://github.com/MaisySen/Mnet/actions/runs/43813517063). The Go job ran an isolated real Chrome flow against the signed fake provider and real disposable control server with explicit membership: sign-in, own-device render, populated sign-out, delayed valid response suppression and server-side token replay denial. Screenshots were inspected locally. Native HTTP/session checks and three kernel VPN regression fixtures also passed, with 159/153/173 later loss-profile drops. The kernel suite is not account UI proof; no live provider/deployment changed.

## Member edge-state Chrome proof (verified extension)

All five jobs passed in [229c0a3 CI](https://github.com/MaisySen/Mnet/actions/runs/43814532649), including the real Chrome member step. It adds literal HTML-like names, empty/denied display, failed logout and 390px no-overflow checks through isolated browser response fixtures, then completes the real isolated-server logout/replay test. Narrow/empty screenshots were inspected. Full kernel VPN regression fixtures passed with 138/142/127 later loss-profile drops. This is not physical mobile proof or a live account test.

## Provider-change revocation (verified source)

All five jobs passed in [b455910 CI](https://github.com/MaisySen/Mnet/actions/runs/43815622508), including native permanent session/pending-login invalidation tests and the real Chrome member step. Full kernel VPN regression fixtures passed with 135/136/153 later loss-profile drops. Reinstalling the old provider cannot revive cleared sessions; keeping the same provider leaves them alone. This is not a live provider change.

## Sign-in failure recovery (verified browser source)

All five jobs passed in [fa00aca CI](https://github.com/MaisySen/Mnet/actions/runs/43816760633), including native account checks and real Chrome rejection-page/return-navigation validation without private callback echo. Three full kernel VPN regression fixtures passed with 118/108/144 later loss-profile drops. The failure screenshot was inspected. No live provider/account was changed.

## Two-device account summary scope (verified browser extension)

All five jobs passed in [7299060 CI](https://github.com/MaisySen/Mnet/actions/runs/43817883961), including real Chrome signed-provider login with two owned device records and same-email/different-subject, other-network, other-issuer and expired outsiders. Actual JSON plus DOM contain exactly the two owned approved/unexpired summaries despite attempted query widening. Desktop/narrow screenshots were inspected. Full kernel VPN regression fixtures passed with 155/123/147 later loss-profile drops. These are control summaries, not two new physical running VPN devices.

## Operator membership inspection (verified browser/native source)

All five jobs passed in [e25b70d CI](https://github.com/MaisySen/Mnet/actions/runs/43819448319). Real Chrome uses the actual disposable operator login, explicit grant API and exact-network read API, checks HTML-as-text and an empty second network, then holds responses across network selection and logout. Native tests reject currently valid account/node credentials in the operator cookie slot and check empty/missing/unknown network responses. Three full kernel regressions passed with 163/127/153 later loss-profile drops. No live membership was changed. The next response-validation extension adds browser-local failure/malformed/reversed-request cases; all five jobs passed in [c834830 CI](https://github.com/MaisySen/Mnet/actions/runs/43820460941). The preceding 90f633b full race job detected an unrelated shared trace-test flag race. A follow-up removes that global test switch; 30 local trace/TCP race repetitions and a fresh full suite/vet pass, and all five jobs passed in [03b55a6 CI](https://github.com/MaisySen/Mnet/actions/runs/43821694030). The strengthened [b3f313d CI](https://github.com/MaisySen/Mnet/actions/runs/43821716633) also passed all five jobs, including 30 Linux trace/TCP race repetitions and native three-repetition trace/TCP tests; kernel loss-profile drops were 152/185/158.

## Operator removal versus account and node access (fixture extension)

The signed-provider account HTTP test now removes membership through the actual guarded operator endpoint rather than calling the store directly. A cross-origin rejection must leave the active account query usable. Accepted removal must deny that query while the original approved node token still authenticates `/api/map` with unchanged ID and approval. Three local race repetitions and a fresh full race suite/vet pass. All five jobs passed in [f81b124 CI](https://github.com/MaisySen/Mnet/actions/runs/43825558997), including the native Windows/macOS account test batch. Three complete kernel regression fixtures passed with 129/123/150 later loss-profile drops. This is HTTP authorization-layer coverage, not VPN reachability or live account work.

## Member response validation (browser extension)

The member page now validates the entire list and each ID/name/address field before creating a complete row set. Local real Chrome checks malformed/null lists/rows, wrong types and a partly valid response without partial render or false empty success, plus reversed completion of two refreshes. Edge data is browser-local; actual OIDC login/device reads/logout still use the disposable service. Fresh full race/vet checks pass. All five jobs passed in [f1e98c1 CI](https://github.com/MaisySen/Mnet/actions/runs/43829486186), including the actual account browser step after the startup harness follow-up below.

## Isolated Chrome startup failure and diagnostics

[3089c84 CI](https://github.com/MaisySen/Mnet/actions/runs/43828177972) passed native, session and kernel jobs but failed before the account browser step: the dashboard harness could not discover its isolated Chrome target within 15 seconds. The browser stderr was previously discarded, so the underlying startup cause is not established. New fixture source retains only a bounded 4KiB startup diagnostic tail, avoids shared-memory dependence and allows 30 seconds for target discovery; test assertions and production timeouts are unchanged. Both local browser fixtures pass. All five jobs passed in [f1e98c1 CI](https://github.com/MaisySen/Mnet/actions/runs/43829486186), including both real-Chrome flows and three kernel regressions with 135/134/142 later loss-profile drops. The earlier startup cause remains unknown; this green run does not prove it cannot recur.

## Committed account logout with a lost response (browser extension)

The actual signed-provider fixture signs in again, commits server-side account logout and drops only the response. The UI must retain uncertainty with private rows cleared; old-token replay must fail, and an idempotent actual logout retry must confirm sign-out even though the first logout removed its cookie/session. All five jobs passed in [ec2892d CI](https://github.com/MaisySen/Mnet/actions/runs/43830783982), including the actual account browser step, native tests and three full kernel regressions with 139/155/142 later loss-profile drops. This is test-only, not a new live account action.

## Account login input non-allocation (fixture extension)

New local three-repetition race tests send missing/null/wrong-type network IDs, unknown principal fields, trailing JSON and oversized bodies through the actual handler. Each must reject without allocating pending state, account sessions or cookies. Known and unknown network IDs with valid input shape both start pending logins without issuing an account session, preserving the existing network-existence boundary. All five jobs passed in [371c558 CI](https://github.com/MaisySen/Mnet/actions/runs/43832312077), including native three-repetition account cases, both actual browser steps and three full kernel regressions with 163/141/169 later loss-profile drops. No production behavior or live accounts changed.

## Unambiguous account login selector (new source)

Account login start now parses exactly one case-sensitive `network_id` field. Duplicate identical/conflicting keys, alternate-case keys and mixed-case duplicates reject before pending state/cookies/session allocation. Local three-repetition signed HTTP/state tests, actual account Chrome and full race/vet pass. All five jobs passed in [43538fc CI](https://github.com/MaisySen/Mnet/actions/runs/43833820901), including native account/state tests, both actual browser steps and three full kernel regressions with 155/172/141 later loss-profile drops. No live changes or new membership authority.

## Unambiguous operator membership writes (new source)

The guarded operator POST now parses each exact case-sensitive field once. Tests reject duplicate decision, same-subject duplicate, conflicting network/issuer and alternate/mixed-case principal/decision fields without changing membership. Existing strict body/size/trailing/type checks and real-Chrome operator removal remain intact. Local three-repetition membership/signed HTTP tests, actual dashboard Chrome and full race/vet pass. All five jobs passed in [4804938 CI](https://github.com/MaisySen/Mnet/actions/runs/43835322107), including native membership cases, both actual browser steps and three complete kernel regressions with 140/172/149 later loss-profile drops. No live changes.

## Unambiguous private membership reads (new source)

Private grant reads now require exactly one case-sensitive network selector and reject malformed query encoding. Native-targeted tests reject duplicate identical/conflicting network IDs, alternate-case/unknown fields and invalid escape/semicolon queries with no private grant output. Local three-repetition membership tests, actual dashboard Chrome and full race/vet pass. All five jobs passed in [8d6727d CI](https://github.com/MaisySen/Mnet/actions/runs/43836834052), including native membership cases, both actual browser steps and three full kernel regressions with 132/121/138 later loss-profile drops. No live changes.

## Authenticated member network context (new source)

The device-summary response includes only its authorized session network ID, beside existing summaries. The member page renders it as text and clears it with private state. HTTP tests attempt query widening and assert the original network; internal tests deny missing/revoked credentials without any context. Actual Chrome checks the signed-in network and its sign-out cleanup. All five jobs passed in [a19afcf CI](https://github.com/MaisySen/Mnet/actions/runs/43838497515), including native account tests, actual Chrome and three full kernel regressions with 141/135/152 later loss-profile drops. No principal/role fields or new membership permissions.

## Member context cleanup edges (browser extension)

Local actual Chrome now explicitly checks the network ID is cleared alongside device rows after denied/malformed reads, failed logout and committed logout with a lost response. Missing/null/wrong-type/empty network context rejects rather than producing a signed-in view. These response edges are browser-local; real login/logout and initial session reads remain server-backed. All five jobs passed in [fe97e6c CI](https://github.com/MaisySen/Mnet/actions/runs/43840039637), including actual Chrome and three full kernel regressions with 143/153/155 later loss-profile drops.

## Explicit operator grant form (new source)

New source lets an operator manually enter the exact issuer/subject from trusted provider records, acknowledge that check and review the selected network/principal before sending the existing guarded grant POST. No email/approval autofill or inference. Actual local Chrome checks cancel leaves the server unchanged, missing acknowledgement cannot write, confirmed grant appears in the actual list and private input clears. All five jobs passed in [618d602 CI](https://github.com/MaisySen/Mnet/actions/runs/43841705310), including actual dashboard Chrome, native checks and three full kernel regressions with 183/162/136 later loss-profile drops. Source fixture only, no live grant.

## Explicit grant uncertain outcome and private draft cleanup (fixture extension)

Local actual Chrome now invalidates a grant confirmation by changing network context and verifies no write/private draft remains. A separate real fixture grant commits but loses only its response; the UI retains uncertainty and fresh actual inspection reveals the grant, with no automatic retry. A populated issuer/subject/acknowledgement draft also clears on sign-out, even while a stale state response is held. All five jobs passed in [50646a5 CI](https://github.com/MaisySen/Mnet/actions/runs/43843273639), including actual dashboard Chrome and three full kernel regressions with 131/146/132 later loss-profile drops. No live grants.

## Reviewed operator grant to member access and removal (browser journey)

The signed-provider Chrome fixture now starts with no membership. It signs into the disposable operator dashboard, checks an empty grant list, submits the reviewed manual issuer/subject grant, follows the landing member entry and signs in through the real signed provider to see exactly its two owned summaries. Later it uses the existing operator session to remove the actual grant; an already active member query must return 401 and revisiting the member page must clear rows/network context and offer sign-in. All five jobs passed in [5e6e406 CI](https://github.com/MaisySen/Mnet/actions/runs/43844911937), including the actual browser journey and three complete kernel regressions with 149/131/174 later loss-profile drops. These are disposable records and one browser journey, not physical VPN devices or a live grant.

## Read-only Linux service unit generation (new source)

`mnetd -service print-unit` prints a saved-enrollment systemd unit only. Tests reject non-absolute/non-canonical/expanding paths, unsafe interface names, secret/enrollment/routing/session/validation overrides and install/remove/run actions. Local actual CLI output passed `systemd-analyze verify`; full race/vet and cross-build checks pass. All five jobs passed in [c3caba0 CI](https://github.com/MaisySen/Mnet/actions/runs/43846988647), including the actual CLI/syntax step and three complete kernel regressions with 117/139/135 loss-profile drops. This is syntax/generation proof, not service installation, automatic boot, reboot or VPN convergence. No host service was installed or enabled.

## Actual Linux systemd kernel fixture (verified)

The first of three CI kernel fixtures now requests `MNET_KERNEL_SYSTEMD=1`; the other two retain the process-only regression. Opt-in requires an actual systemd PID 1 on a disposable root Linux runner. A unique runtime-only `/run/systemd/system/mnet-ci-*.service` uses the generated production ExecStart unchanged, plus test-only existing network-namespace and private hosts-file isolation. It starts the already-enrolled client, requires authenticated Up, the same saved node/key/IP, direct peer path, installed client route and real LAN traffic, then checks stop/TUN/socket cleanup, clean start and SIGKILL restart-on-failure with a new main PID and restored VPN/LAN. Cleanup stops/removes the test unit before network namespace deletion. No unit is enabled; no physical boot/reboot, live network or public-internet proof. All five jobs passed in [6fc9839 CI](https://github.com/MaisySen/Mnet/actions/runs/43847999930). Logs explicitly confirm both systemd stages; all three full kernel regressions passed with later loss-profile drops 141/134/136.

### Clean process shutdown listener completion (new source)

Preparing actual systemd validation reproduced a clean-exit ordering bug: a stopped offline daemon returned zero but left its Unix socket file in 30/30 fresh-process checks. The new in-process regression failed before the fix. Console/service shutdown now waits for the local API goroutine after cancellation and Down. Thirty race-test repetitions (ten lifecycles each) and 30 fresh actual process checks pass locally with no socket remnants; full source/native checks passed in [6fc9839 CI](https://github.com/MaisySen/Mnet/actions/runs/43847999930), including 30 macOS repetitions and Windows SCM lifecycle. This is clean process shutdown proof, not forced-crash file cleanup or physical boot proof.

## Linux daemon state ownership (verified)

Two actual offline processes previously stayed alive on one state directory, and the second replaced the first local API socket. Linux `mnetd` now holds a nonblocking kernel flock before loading/initializing identity or opening the API. The private stable `daemon.lock` inode is never deleted; regular-file/one-link checks and no-follow open reject aliases, directories and FIFOs. Process exit/crash releases the descriptor lock, not a PID/timestamp expiry. Thirty local race repetitions cover duplicate owners, independent directories, release/reacquire and rejected paths with unchanged alias targets. A fresh-process script rejects five duplicates while preserving the original live API socket/token, then verifies clean stop and SIGKILL permit restart without replacing the lock inode. All five jobs passed in [9b6c459 CI](https://github.com/MaisySen/Mnet/actions/runs/43849057244), including the actual-process script, systemd VPN recovery and three complete kernel regressions with 158/157/148 later loss-profile drops. Guard applies only to Linux console/service `mnetd`, not mobile, other operating systems, copied identities in different directories, network filesystems or a malicious local administrator replacing paths.

## Read-only macOS launchd configuration (verified)

`mnetd -service print-plist` prints XML only. Shared conservative saved-state path/override validation retains Linux generator behavior; at that revision macOS additionally required a canonical explicit `utunN`, because the daemon then configured the requested name rather than propagating auto-assigned utun names. The newer automatic-selection extension is recorded below. Local repeated generator/XML tests and fresh full race/vet/cross-builds pass. All five jobs passed in [f2862f7 CI](https://github.com/MaisySen/Mnet/actions/runs/43851053165). Native CI executed the actual Darwin command, compared exact argv/key structure with Python plistlib, ran Apple's plutil lint and rejected secret/unsupported options without state creation. Three complete kernel regressions passed with 170/142/156 loss-profile drops. No launchctl call or job loading occurs. Generated RunAtLoad/failure-only KeepAlive/throttling/decimal umask follow the [Apple format reference](https://github.com/apple-oss-distributions/launchd/blob/main/man/launchd.plist.5); these tests are not launchd recovery, actual macOS VPN, interface-availability or boot/reboot proof.

## Actual macOS launchd offline lifecycle (verified)

A root-only disposable native GitHub Actions runner script generates the plist and changes only its label to a unique test identifier, leaving production argv/recovery settings intact. It never touches a production label or /Library configuration, or changes persistent enable/disable settings. System-domain bootstrap should start the offline authenticated local API (Down, not VPN-ready); unauthenticated access must fail. SIGKILL should trigger automatic failure-only restart with a different process PID and the same local API token. Bootout must remove process/socket, then reload and final unload must preserve the token and remove the job. Cleanup checks even a partially registered job. All five jobs passed in [586062e CI](https://github.com/MaisySen/Mnet/actions/runs/43852789042). Native logs explicitly confirm startup, crash recovery and final unload/reload cleanup. Three full Linux kernel regressions passed with 152/133/129 later loss-profile drops. No enrollment/control service, actual macOS VPN/interface, boot/reboot or physical-host proof. [Command reference](https://keith.github.io/xcode-man-pages/launchctl.1.html) grounds bootstrap/bootout/kill semantics; unstable launchctl print text is not parsed.

## macOS state ownership extension (verified)

The same Linux no-follow regular-file/one-link/flock ownership guard now builds on Darwin and runs before console/service identity and API setup. Linux repeated race/alias tests, actual-process preservation/simultaneous contenders and clean/crash release checks still pass; full local race/vet and Windows/Darwin cross-builds pass. Native macOS CI independently repeated lock tests 30 times and ran actual duplicate/API-token/socket preservation and clean/SIGKILL restart checks, then rechecked the actual launchd lifecycle with ownership enabled. The lock file is never deleted. All five jobs passed in [0e4d592 CI](https://github.com/MaisySen/Mnet/actions/runs/43854588310), with explicit native process and launchd logs and three full Linux kernel regressions (176/154/159 later drops). Windows/mobile, copied identities in separate directories and hostile administrator path replacement remain outside this guard.

## Actual macOS kernel utun setup (verified)

A root-only disposable native GitHub Actions script creates one fresh node against an isolated loopback control plane. It chooses an absent explicit utun interface, rejects existing mesh-range routes and never changes defaults. After authenticated Up, it requires the real kernel interface's assigned mesh address/MTU and mesh route. Down/Up and clean process restart with saved state must retain node/key/token/IP and restore the utun route; final Down/process teardown must remove the interface/route/socket. Fixture credentials remain local and are not artifacts. All five jobs passed in [310bf59 CI](https://github.com/MaisySen/Mnet/actions/runs/43856374974), with explicit native kernel setup/teardown logs and three full Linux kernel regressions (135/137/143 later loss-profile drops). This is one-node host setup, not two-host encrypted peer traffic, LAN/exit forwarding, public internet, physical boot/reboot or physical-device proof.

## Enrolled macOS launchd kernel lifecycle (verified)

The real-utun fixture now continues from cleanly stopped saved enrollment into a generated system-domain launchd job. Only its label is unique; argv/recovery settings remain generated, no /Library installation or persistent enable change. Bootstrap, automatic SIGKILL recovery with a new PID and unload/reload must each restore the same kernel address/MTU/mesh route, node/key/token/IP and API token. Final bootout must remove job/process/socket/interface/route; failure cleanup covers partially loaded jobs. The route guard now rejects any parsed non-default IPv4 route overlapping the full mesh /10, including broader and host routes. All five jobs passed in [3960f8b CI](https://github.com/MaisySen/Mnet/actions/runs/43858139526), with explicit native launchd/kernel recovery logs and three full Linux kernel regressions (172/124/148 later loss-profile drops). Not encrypted peer VPN traffic, LAN/exit routing, OS boot/reboot or physical-device proof.

## macOS kernel-to-userspace peer TCP (verified)

A proof-only Go test process joins the same disposable network on its own userspace TUN. It listens only inside proofnet, not on an OS TCP port. Native macOS OS TCP must cross its actual utun, WireGuard encryption and this peer to deliver 4MiB and receive the peer-computed SHA256 ACK. The fixture repeats after Down/Up, saved process restart and generated launchd bootstrap, automatic forced-crash recovery and reload. Private config/readiness stays local; no credentials in arguments/artifacts. All five jobs passed in [a4ac5de CI](https://github.com/MaisySen/Mnet/actions/runs/43860049583), with native initial/recovery kernel TCP hash/ACK logs and three Linux kernel regressions (150/144/137 later loss-profile drops). This is a same-host hybrid kernel/userspace fixture, not a second kernel host, physical-device/public-internet test, direct-path/offline-control guarantee or physical boot.

## macOS bidirectional direct TCP without control/relay (verified)

The same-host hybrid fixture now requires 4MiB of distinct data in each direction, with SHA256 computed by each receiver and acknowledged by its sender. Every existing recovery stage repeats it. Then actual peer status must report online/non-relay endpoint while traffic flows, after which the disposable control/relay process is terminated and awaited. Three new TCP sockets must transfer 4MiB each way with both hash ACKs without that process. Local protocol race cases cover reverse data/hash ACK and rejecting a wrong reverse hash, plus original truncation checks. All five jobs passed in [e64d449 CI](https://github.com/MaisySen/Mnet/actions/runs/43861794260), including explicit native bidirectional/offline-control transfer logs and three Linux kernel regressions (134/142/133 loss-profile drops). This does not prove two kernel hosts, public internet/NAT traversal, long offline authorization, physical boot or physical devices.

## macOS received authorization denial (verified)

After the existing lifecycle transfers, stop the disposable control and set only the enrolled kernel node approved=false in its private snapshot, then restart that control. The running macOS daemon must receive denial, show waiting-approval and no peers. Three fresh OS TCP connections to the proof-only encrypted peer must fail. Saved node/key/token/IP and local API token remain unchanged. Reauthenticate the fixture administrator, approve that exact node and require Up with no stale approval link plus restored bidirectional 4MiB hash/ACK. Existing direct/offline-control transfers then still run. All five jobs passed in [bbb8bb8 CI](https://github.com/MaisySen/Mnet/actions/runs/43863430835), with explicit native denial/reapproval logs and three Linux kernel regressions (145/127/159 later loss-profile drops). This is received denial, not discovery of unseen offline revocations, fleet authorization proof or two physical hosts.

## macOS known key expiry with control offline (verified)

Stop the disposable job/control, give its existing node a 45-second future key deadline in the private stopped snapshot and restart control. A fresh generated launchd Up must consume that map. Direct bidirectional transfers must succeed before the deadline with control/relay stopped. While the process remains stopped, the known deadline must cause key-expired/no peers and three failed fresh OS TCP connects, preserving saved identity and API token. Final unload still removes job/process/socket/interface/route. All five jobs passed in [7e573f0 CI](https://github.com/MaisySen/Mnet/actions/runs/43865116996), with explicit native known-expiry/offline denial logs and three Linux kernel regressions (141/142/133 later drops). Not unseen offline administrator changes, long-duration/fleet proof or two kernel hosts.

## Automatic macOS kernel interface selection (verified)

The console and shared engine now default to automatic utun on Darwin rather than Linux mnet0. After creation the engine reads and validates the actual kernel utunN before calling host configuration, publishes that actual name in local status and clears it on teardown. It does not replace the requested automatic name with the last assignment, so later creations may safely get different names. Explicit names and other platforms retain prior behavior. The read-only plist generator now accepts automatic utun and the actual native CLI default check expects it. A second full native fixture omits console/plist ifname flags, follows actual status names on each Up and repeats kernel address/route, encrypted peer TCP, recovery, received denial, known expiry and cleanup. Ten local race repetitions for name/status/generator tests, fresh full race/vet and Darwin/Windows cross-builds pass; all five jobs passed in [a4eae56 CI](https://github.com/MaisySen/Mnet/actions/runs/43867041361). Native logs independently show explicit and automatic modes passing kernel/peer/recovery/denial/offline-expiry/cleanup; three Linux regressions passed with 153/147/163 drops. No live host installation or physical boot proof.

## Invalid macOS managed interface before identity/enrollment (verified)

Managed Darwin constructor validates automatic utun or a canonical uint16-range utunN before loading state. This prevents malformed requests such as mnet0, leading-zero names, suffix junk or overflowing indices from reaching enrollment and then failing at kernel setup. Errors do not echo arbitrary invalid input. Unmanaged supplied mobile/proof TUNs bypass this host-name check; non-Darwin behavior is unchanged. Local ten-repetition validation race cases, full race/vet and Darwin/Windows cross-builds pass. Native constructor tests require no state-directory creation. Actual Darwin CLI cases use invalid names with Up/server flags and require exit before identity/token/socket creation. [e1ce7fc CI](https://github.com/MaisySen/Mnet/actions/runs/43868888841) passed these constructor/actual CLI checks and both full valid-interface kernel regression modes, plus three Linux kernel runs (152/150/165 drops). Its native job later failed an older offline teardown fixture that hardcoded Linux mnet0 on Darwin and waited only for readiness, masking the new constructor refusal until timeout. The correction uses the platform default and surfaces early daemon exit directly; thirty local race repetitions and Darwin test cross-compilation pass. The corrected [43190c8 CI](https://github.com/MaisySen/Mnet/actions/runs/43870595932) passed all five jobs, including thirty native clean-stop repetitions, invalid-name constructor/CLI checks and both full native kernel modes; three full Linux kernel runs passed (135/156/150 loss-profile drops). The earlier e1ce7fc remains a failed run, not retroactively green.

## Human CLI actual kernel interface (verified)

Ordinary human status now prints the actual managed kernel interface when present, so automatic utun users can inspect it without JSON. Down and legacy/missing-interface output do not invent a name. JSON retains the same actual interface field. Ten local CLI race repetitions cover actual/legacy/down/approval cases and JSON round-trip, with fresh full race/vet and cross-builds. Both native explicit/automatic kernel fixtures now invoke the actual CLI and compare human/JSON interface and address against the live kernel, initially and after generated launchd recovery/reload; saved secret fields must remain absent. All five jobs passed in [60e7adc CI](https://github.com/MaisySen/Mnet/actions/runs/43872470943), including both explicit/automatic native kernel CLI comparisons and three Linux regressions (123/141/128 drops). Interface existence alone is still not peer reachability.

## Android startup configuration preflight (verified JVM policy)

The foreground VPN worker now snapshots and validates saved server/network plus legacy exit selection before constructing a Go node. Missing configuration and unsupported exit routing fail with fixed errors, not private values. Empty auth keys remain valid for manual approval/saved identity. The dormant default-route builder branch is removed. If Android refuses establish, the worker now enters the common failure cleanup instead of leaving running/shared-node state until destruction. A plain-JVM test covers normalization, missing/null/blank fields, optional auth/name and unsupported exits without secret echo. Android34 aapt2 resources/manifest and every app Java source compiled locally against the existing generated Go binding jar; that does not verify current Go AAR contents, VPN runtime, cleanup under actual Android, consent or reboot. All five jobs passed in [5105980 CI](https://github.com/MaisySen/Mnet/actions/runs/43874486837); JVM policy logs pass, native Android runtime unverified. [Product scope](product-scope.md) records OS-managed always-on guidance and remaining lifecycle/UI gaps.

## Android VPN general-log privacy (verified source policy)

The VPN worker no longer writes configured server/network, assigned IP, full node status JSON or caught exception stacks into general Android logs. Those values can contain private approval or transport URLs. Fixed lifecycle messages remain; detailed status and error text stay in the app-scoped status flow. A new source-only policy test rejects nonliteral VPN Log messages and exception/stack arguments, with regression examples for private status, server, keys and failures. Local policy tests and full Android34 app Java compilation against existing bindings pass. No runtime log capture or physical Android evidence. The preceding startup-preflight [5105980 run](https://github.com/MaisySen/Mnet/actions/runs/43874486837) completed all five jobs. [d4ba818 CI](https://github.com/MaisySen/Mnet/actions/runs/43876328009) also passed all five jobs, including both Android policies and three Linux kernel runs (140/126/154 drops).

## Android single-worker ownership (verified JVM policy)

A process-wide ticket gate refuses a second VPN worker until the owner actually exits; canceling alone does not release ownership. The worker uses a local node reference, checks cancellation before node publication/after Prepare/after establish, closes an unpublished interface and always stops its local node before releasing ownership. Shared-node cleanup compares identity before clearing; a failed foreground/thread startup releases its ticket. Local JVM tests cover cancellation, stale-ticket isolation and 100 rounds of sixteen simultaneous starts with exactly one winner. Full Android34 Java compilation against existing bindings passes. This is a bounded non-overlap/stale-publication change, not complete cancellation correctness: stop immediately before entering a blocking Go call still needs a stronger cross-language cancellation boundary. All five jobs passed in [9c05393 CI](https://github.com/MaisySen/Mnet/actions/runs/43878415619), including JVM ownership cases and three Linux kernel regressions (154/161/128 drops). No physical Android startup/reconnect/always-on proof. The subsequent source extension below addresses the identified cross-language disposal gap.

## Permanent node disposal across Android/Go (verified native cancellation)

Shared-engine Close marks closed under its status lock before Down. Prepare/Up/SetTUN refuse a disposed node; Up rechecks before publishing its cancellation context, so Close before that publication cannot be lost. Close during blocked Prepare/Up cancels it, waits for teardown and cannot be undone by later calls. Stop/Down remain reusable. Android worker stop/finally now call permanent Close; canceled workers holding a local reference cannot revive that same node at a later call. Mobile rejected Start/StartFlow also close supplied tunnel adapters, covering the Close-between-injection-and-Up path. Repeated local race tests cover permanent refusal, blocked Prepare/Up, thirty startup races per test and consumed-FD closure. Fresh full race/vet/Darwin/Windows builds passed before the additional tests, which also pass. A new arm64 Android Go AAR was generated from current source, inspected for Node.close and used to compile all app Java sources against Android34. This is a private build check, not a published APK/AAB, other-ABI build, Android VPN runtime or physical always-on/reboot proof. All five jobs passed in [3932c31 CI](https://github.com/MaisySen/Mnet/actions/runs/43880900442), including ten native macOS/Windows cancellation repetitions and three Linux kernel regressions (120/138/154 drops). Physical Android remains unverified.

## Pending injected-tunnel ownership (verified native source)

Two fresh regressions failed before this follow-up: Close did not close an injected tunnel before Up consumed it, and a second SetTUN silently replaced the pending one. Injection now shares the lifecycle lock, refuses nil and replacement while a pending tunnel is owned, and transfers the pending slot to the device attempt when consumed. Down/Close close and clear unconsumed pending tunnels; a fresh injection after Down works. Mobile Up-failure cleanup calls Down before closing its adapter, releasing any pending slot. Repeated local race tests check exactly-once pending closure, caller ownership on rejected replacement and Down/replacement cleanup, plus previous permanent-close/FD cases. Full fresh race/vet/Darwin/Windows builds pass. All five jobs passed in [7ddee3f CI](https://github.com/MaisySen/Mnet/actions/runs/43883114265), including ten native macOS/Windows repetitions and kernel loss runs159/139/125. No physical Android/runtime or new published artifact claim.

## Android API-level entry points (verified JVM/source policy)

The app declares minSdk24 but used API26 notification-channel calls and the flags receiver overload without lower-version paths. A shared helper now uses legacy low-priority notifications on API24/25 and creates/reuses channels on API26+. Both VPN and session-host notifications use it. Both activity status receivers use a signature-only app permission on every version; API33+ additionally uses RECEIVER_NOT_EXPORTED. The legacy path is not an unprotected receiver.

The real helper passes an API24-36 branch matrix compiled against API-checking JVM fakes. Source checks require both receiver/notification call sites and the manifest permission. This is fake/source coverage, not framework execution. All five jobs passed in [d09bf3f CI](https://github.com/MaisySen/Mnet/actions/runs/43885644442), including the JVM-fake matrix. A freshly rebuilt current-source arm64 Go AAR, every Java source/resource against Android34, and min-api24 dex compilation pass locally. Other ABIs, actual Android24/25 startup, current Android VPN/session behavior, consent, always-on and reboot remain unverified. No APK/AAB published.

API basis: [Notification.Builder constructors](https://developer.android.com/reference/android/app/Notification.Builder), [Context receiver overloads](https://developer.android.com/reference/android/content/Context) and [signature permissions](https://developer.android.com/guide/topics/permissions/defining).

## Android projection-stop lifecycle (verified JVM/source policy)

Target34 session hosting lacked the MediaProjection onStop callback required before virtual-display creation. A ProjectionGuard registers first and handles app/system stop once, unregistering without recursive notifications. System capture stop now stops the host, disables session input, drops retained frames and releases display/reader/projection/thread/foreground state. Setup failures use the same cleanup. Duplicate starts do not consume another consent token; the service is non-sticky because projection consent cannot be replayed. The host worker checks cancellation under a lock shared with teardown; teardown releases that lock before waiting for Go workers. Only a successfully started host is stopped. The hosting activity also uses the legacy startService entry point below API26.

All five jobs passed in [841b83b CI](https://github.com/MaisySen/Mnet/actions/runs/43887988825). JVM fakes cover callback-before-display, null refusal, external/app-stop idempotence and100 concurrent stop races. Source checks cover service callback order, release paths, non-sticky/duplicate-start policy and retained-frame gates. All Java sources compile against Android34 with the recently rebuilt current-Go arm64 AAR; min-api24 dex compilation passes. These checks do not run Android, grant consent, capture a screen or exercise the actual Java/Go host teardown. Rotation/resizing, system stop/lock, repeated consent/start/failure and physical session/VPN tests remain open. No new APK/AAB published.

API basis: [Android projection lifecycle](https://developer.android.com/media/grow/media-projection), [MediaProjection callback requirement](https://developer.android.com/reference/android/media/projection/MediaProjection) and [Context service entry points](https://developer.android.com/reference/android/content/Context).

## Mobile host ownership and late consent (verified native loopback)

A fresh regression failed before the fix: StopSessionHost on an unrelated Node closed another node's process-global listener. A host run now owns its listener, pending accepted connection and active session under an explicit Node identity. Only that owner can stop it. Stop closes accepted connections even while handshake/consent blocks; a late consent response cannot publish a session. An old worker's cleanup cannot remove a replacement run. Stopping active host workers happens outside the shared mutex. Node Stop/Close also stop their owned host; Close permanently refuses later host publication. This remains one process-wide mobile host, not one simultaneous host per node.

Thirty local race repetitions cover wrong-node stop isolation, a real loopback session handshake blocked on consent, immediate accepted-connection closure, late consent/old-run exit with replacement survival, disposed-node refusal and active session capture/mux/worker shutdown. Full fresh race suite/vet/Darwin/Windows builds pass; all five jobs passed in [5647bf6 CI](https://github.com/MaisySen/Mnet/actions/runs/43890650925), including30 native macOS/Windows loopback repetitions. A freshly rebuilt current-source arm64 Android AAR compiles all app Java and min-api24 dex. These are engine loopback/build checks, not Android VPN/capture/consent or physical Java/Go teardown proof. App callbacks already blocked in user code cannot be forcibly canceled; the pending transport is closed and the old worker exits when the callback returns. No APK/AAB published.

## Mobile host continues after peer refusal (verified native loopback)

A fresh real-loopback regression reproduced an ordinary-use failure: refusing one session terminated the host listener, preventing a later approved viewer. A failed accepted-peer handshake is now distinguished from a listener failure. The accepted connection is closed and the owner keeps accepting unless it was stopped; true listener errors still end that run. A malformed peer follows the same cleanup path.

Thirty local race repetitions pass a refused peer, malformed peer, then approved session on one listener, plus previous owner/late-consent/active-stop cases. Full fresh race/vet/cross-build and current-source arm64 AAR/full Java/min-api24 dex checks are recorded locally; all five jobs passed in [d721025 CI](https://github.com/MaisySen/Mnet/actions/runs/43892895603), including30 native macOS/Windows mobile repetitions. No Android framework, physical capture, public-internet load or malicious-peer resilience claim. This does not add a handshake timeout or prevent a connected silent peer from waiting for its first frame. No APK/AAB published.

## Mobile host hello deadline (verified native loopback)

Mobile accepted connections now have a15-second initial hello deadline. A valid hello switches to a70-second consent/reply transport window, allowing the Android prompt's existing60-second wait. After an accepted handshake, the deadline is cleared before active-host setup. A silent peer no longer occupies the initial read forever; timeout closes that accepted connection and hosting continues. This bounds transport I/O, not the duration of an already-blocked application callback.

Thirty local real-loopback race repetitions use a short injected hello deadline to require server-side silent-peer closure, later approved-peer recovery and an active mux surviving beyond the short hello deadline. Removing deadline enforcement makes the regression fail. Full fresh race/vet/Darwin/Windows cross-builds pass, with current-source arm64 AAR/full Java/min-api24 dex build checks. All five jobs passed in [d5522ed CI](https://github.com/MaisySen/Mnet/actions/runs/43895158992), including30 native macOS/Windows mobile repetitions. This is not public-internet denial-of-service, concurrent-client fairness or Android framework/capture proof. No new APK/AAB published.

## Mobile viewer transport ownership (verified native in-memory transport)

After its existing10-second TCP dial, a mobile viewer now gets a70-second session hello/reply I/O deadline. Failure closes the connection and removes its ownership entry. Acceptance clears the deadline before active viewing. A Node tracks its pending and active viewer connections; Stop closes them without permanently disposing the node, while Close also prevents later viewer publication. Another node's stop does not close them. Callback delivery follows mux closure and removes the entry. This does not interrupt a TCP dial already in progress; its existing10-second limit still applies, and a disposed node refuses the returned connection.

Thirty local race repetitions use in-memory full-duplex net.Pipe transports for silent-host timeout/closure, Close during a blocked reply, disposed-node refusal, active deadline clearing, unrelated-node isolation and owner-stop/one close callback. Earlier mobile host tests continue using actual OS loopback TCP. Full fresh race/vet/Darwin/Windows builds and a newly rebuilt current-source arm64 AAR/full Java/min-api24 dex build pass. All five jobs passed in [e12ce56 CI](https://github.com/MaisySen/Mnet/actions/runs/43897494731), including30 native macOS/Windows mobile repetitions. This does not test Android Activity destruction/reconnect callbacks, physical viewing, screen decode or framework/VPN lifecycle. No new APK/AAB published.

## Android viewer attempt lifecycle (verified JVM/source policy)

An Activity-local attempt gate refuses overlapping connects and scopes callbacks/publication to one attempt. A viewer returned after destruction or after an earlier close is rejected and closed, rather than published into the dead Activity. Main-thread transitions deduplicate reconnect timers; stale frame/chat/closed callbacks cannot affect a replacement. Send workers keep the selected viewer reference rather than reading a later replacement. Destruction permanently closes the gate, closes its current viewer and removes only the retry runnable, leaving late queued publication cleanup able to close its returned handle.

Local JVM tests cover exclusivity, stale callbacks/publication, replacement isolation, permanent close and1000 concurrent publish/destroy races. Source checks require Activity wiring and reject removing every queued handler callback. Every Java source compiles against Android34 with the current-Go arm64 AAR, and min-api24 dex compilation passes. [7a560ee CI](https://github.com/MaisySen/Mnet/actions/runs/43899740458) passed all five jobs, including the JVM gate/source logs. These are JVM policy/source/build checks, not Android Activity execution, reconnect/screen-decode/rotation or physical viewing proof. A connect already waiting on the Go handshake is not canceled by Activity destruction alone; its returned handle is discarded, with the Go reply timeout as the bound. No new APK/AAB published.

## Android exact-request host consent (new source, CI pending)

The old process-global pending answer/latch let a stale dialog answer a later peer request, and an old worker could read a replacement latch. A consent broker now creates an immutable request ID and owner for each prompt. The dialog sends that exact ID for allow/refuse/cancel; expired, canceled or already answered IDs are ignored. A worker waits on its own request. Stop cancels only its service's request, and an old waiter's final cleanup cannot clear a replacement. Missing/stale request broadcasts do not show a new dialog. Existing explicit auto-accept behavior is unchanged.

Local JVM tests cover overlapping-prompt refusal, stale dialog/stop/worker isolation, exact first answer, timeout and 1000 concurrent answer/cancel races. Source checks require ID wiring at every dialog answer and owner-scoped cancellation; full Android34 Java/min-api24 dex compilation passes against current-Go arm64 bindings. [e86a1bc CI](https://github.com/MaisySen/Mnet/actions/runs/43901980153) has four successful jobs; native macOS was canceled before any steps/log and the run is not all-five verified. This does not execute an Android dialog, verify user identity or establish physical capture/consent behavior. A missing UI still times out closed after 60 seconds. No new APK/AAB published.

## Android host-run auto-accept scope (verified source/build policy)

Auto-accept was a process-global flag, which could remain true after a host run and disagree with a newly created unchecked switch. It is now an instance setting defaulting false, carried only by the explicit start intent and cleared on stop. HostingActivity snapshots the switch when requesting screen consent, prevents overlapping capture requests and resets it after the result. A recreated Activity without that pending snapshot defaults false. The switch now says "Auto-accept viewers on next hosting start"; changing it does not change an already running host. Use Stop hosting to revoke the current run, then start again with the desired setting.

Source checks require default-false intent handling, stop/reset and request snapshot wiring. Full Android34 Java/min-api24 dex compilation and existing JVM consent/viewer/projection policies pass. All five jobs passed in [aa728e6 CI](https://github.com/MaisySen/Mnet/actions/runs/43904383406). At that revision the auto-accept wiring had source coverage, not its own executed Activity/service test. The changed switch label/layout is not inspected in an actual Android UI. Physical consent, rotation/recreation, live UI and capture remain unverified. No APK/AAB published.

## Selected Android service framework simulation (verified30 cases)

A repeatable Maven/Robolectric4.18 harness now executes the real selected HostSessionService/SessionConsent/ProjectionGuard/VpnPlatform Java sources on simulated API24/25/26/33/34. Twenty cases passed locally and in all-five [e2d03a6 CI](https://github.com/MaisySen/Mnet/actions/runs/43907146698): notification/channel construction plus receiver registration, non-sticky restart/stop, invalid capture-consent setup cleanup and an actual service consent wait canceled by stop. CI runs the same script; dependencies are pinned and temporary test sources are copied from the current checkout rather than maintained duplicates.

This is framework simulation, not an Android emulator/device. Go Node, VPN entry and accessibility are explicit fakes; manifest processing is disabled. It does not prove real signature-permission enforcement, valid projection capture, Go JNI teardown, Activity dialogs/pixels, auto-accept UI flow, rotation, always-on or physical boot. The auto-accept label/layout remains visually unverified in Android. See [harness setup and exact limits](../android/framework-tests/README.md). No production source, APK/AAB or live service is changed by this test harness.

## Service consent framework extension (verified simulation)

The same framework simulation passed30 cases locally and in all-five [5db95ec CI](https://github.com/MaisySen/Mnet/actions/runs/43909538787). Two added tests per API execute actual host service consent broadcasts and answers: refusing one request then trying its old ID against a new peer leaves the second pending until its own ID is allowed; auto-accept is instance-local and service stop clears it. These tests use reflected running/auto-accept state to isolate the service logic. They do not execute the Activity switch, valid projection start or system consent flow, and are not visual checks. Existing fake Go/VPN/accessibility and disabled-manifest limits still apply. No production source or published artifact changed.

## Capture frame resource cleanup (verified simulation/build)

The ImageReader callback now acquires inside its protected try/finally and closes every acquired Image, including stopped/failed frames. A capture encoder owns and releases padded/cropped temporary Bitmaps explicitly, avoiding accumulating temporary native frame allocations until garbage collection. It avoids recycling the same Bitmap twice when crop returns the original. Unsupported pixel/row layouts reject safely, compression failure is an error and failed frames are not published.

Forty simulated-framework cases pass locally and in all-five [8c6478e CI](https://github.com/MaisySen/Mnet/actions/runs/43911955470) across five API levels. Added encode cases cover padded/exact RGBA rows, decoded JPEG dimensions and invalid/short buffers. Acquisition/Bitmap-release paths also have source wiring checks. Full Android34 Java/min-api24 dex compilation passes against current-Go arm64 bindings. These checks do not measure native Android heap pressure, execute a real ImageReader/capture callback, validate color/orientation from a device, or inspect actual capture pixels. Physical capture, projection size changes and Activity UI remain unverified. No new APK/AAB published.

## Capture visible-row buffer boundary (new source, CI pending)

[Android Image.Plane](https://developer.android.com/reference/android/media/Image.Plane) guarantees bytes only through the last pixel, not trailing padding after the final row. A new direct-buffer regression failed on all five simulated APIs before this fix because whole padded-bitmap copying demanded those extra bytes. The encoder now copies visible row bytes into one exact-size frame, skips padding without reading beyond the final pixel and preserves the source buffer position/limit through a duplicate. Exact rows avoid the packing allocation. Arithmetic/remaining-byte validation rejects truncated or unsupported layouts.

Forty-five local framework-simulation cases pass, including omitted final-row padding and a nonzero source-buffer offset. Full Android34 Java/min-api24 dex and source cleanup checks pass. This verifies synthetic buffers/framework simulation, not actual device ImageReader colors, stride variants, crop/orientation or memory-pressure behavior. No real capture pixels were inspected. [44be5db CI](https://github.com/MaisySen/Mnet/actions/runs/43914849207) has four successful jobs including45 simulated cases; native macOS was canceled before steps and the run is not all-five verified. Published APK/AAB unchanged.

## Android viewer completed taps (4a98e99 all-five CI verified)

The viewer previously translated both finger-down and finger-up into separate clicks, so one tap could activate a host control twice. It now sends one click only on a completed stationary release. Canceled, multi-touch, dragged or letterbox-margin gestures clear the pending tap. A drag that returns to its start is still suppressed. This does not implement drag/scroll/multi-touch forwarding; unsupported gestures must not turn into unintended taps.

Local JVM policy tests pass1000 completed taps with duplicate-up suppression plus cancellation/movement cases. Source checks cover MotionEvent wiring; full Android34 Java/min-api24 dex compiles. No real MotionEvent, accessibility gesture or physical remote-control action is tested here, and no UI layout changes are made. All-five [4a98e99 CI](https://github.com/MaisySen/Mnet/actions/runs/43917164476) passes, including45 service/encoder simulated cases. Published APK/AAB unchanged.

## Viewer Activity MotionEvent simulation (904a1af all-five CI verified)

The framework harness now copies the actual ViewerActivity/ViewerGate/ViewerTap and uses an explicit fake Go viewer transport. Its real asynchronous connection publication path runs, then simulated MotionEvents pass through the actual screen touch listener. Four local Activity cases on simulated API24/25/26/33 require zero clicks on down, one on up, no duplicate-up/cancel/drag clicks and viewer closure on Activity destruction. Together with45 service/encoder cases,49 passed all-five [904a1af CI](https://github.com/MaisySen/Mnet/actions/runs/43919715270).

API34 Activity creation failed with a Robolectric VirtualDeviceManager listener class mismatch before touch execution, so it is explicitly excluded from this Activity test, not reported as tested. Service/encoder simulation still includes API34. The test uses reflection only to inspect its screen/gate/frame state, not to replace lifecycle state. No actual UI pixels were inspected, no real accessibility input or JNI runs, and these are not emulator/device tests. The existing physical/UI/capture limits remain. No production source or published artifact changed.

## Viewer frame input boundaries (2420ae9 all-five CI verified)

The viewer no longer assumes a fictional1x1 frame before receiving screen content. It suppresses input until a decoded frame gives valid dimensions. A frame-size change cancels a pending tap so it cannot release in a different coordinate system. Disconnect clears frame dimensions, displayed content and pending tap before retrying.

Expanded Activity regressions failed on each of simulated API24/25/26/33 before the initial-frame fix, then failed separately on the resize check before its fix. All49 cases pass in all-five [2420ae9 CI](https://github.com/MaisySen/Mnet/actions/runs/43922014681), including synthetic JPEG frame delivery through the actual callback, resize cancellation and disconnect clearing. Full Android34 Java/min-api24 dex compilation passes against current-Go arm64 bindings. These are simulated events and synthetic frames with fake Go transport, not actual capture pixels, physical input, JNI or emulator/device proof. API34 Activity simulation remains excluded as documented. No new APK/AAB, release or live deployment.

## Host capture/display geometry input refusal (a1b761c all-five CI verified)

The Android host now refuses input when frame dimensions are uninitialized or the current display width, height or rotation differs from capture setup. A detected geometry change latches remote input off and logs a notice to restart hosting. It does not dynamically resize/restart capture or claim rotation support. Fresh capture setup resets the latch in source; a real projection restart is still unverified.

Two added service tests across simulated API24/25/26/33/34 use reflected capture state and an explicit fake accessibility sink. Initial-dimension and display-width mismatch assertions each failed before the guard. The fixed tests cover width/height/rotation refusal and the refusal latch.59 framework cases pass in all-five [a1b761c CI](https://github.com/MaisySen/Mnet/actions/runs/43924405581) (55 service/helper,4 Activity); API34 Activity exclusion remains. Full Android34 resources/Java and min-api24 dex compile against current-Go arm64 bindings. No physical display rotation, injected gesture, real projection or JNI executes, and no actual Android UI pixels were inspected. Published APK/AAB and live service are unchanged.

## Android host pointer validation (fa9ec75 all-five CI verified)

The host now checks pointer payloads before forwarding to the accessibility endpoint: only completed click and fixed scroll actions with integer, finite, nonnegative coordinates inside the capture frame are accepted. Missing x/y remain zero for Go omitempty compatibility. Malformed JSON, non-pointer input, string/null/fractional/out-of-frame coordinates and unsupported down/up/move are refused. The accessibility source no longer converts down/up into separate clicks and also checks scale/coordinate finiteness. Older senders relying on that incorrect behavior must send completed clicks instead; held-pointer forwarding is still unsupported.

One added service test across simulated API24/25/26/33/34 failed before validation, then passed its invalid/valid payload assertions after the fix.64 framework cases pass in all-five [fa9ec75 CI](https://github.com/MaisySen/Mnet/actions/runs/43926681146) (60 service/helper,4 Activity), with a fake accessibility sink. Full Android34 resource/Java/min-api24 dex compilation passes against current-Go arm64 bindings. The real accessibility-service changes have source/build checks only, not gesture runtime or physical input proof. API34 Activity simulation remains excluded. No release artifacts or deployment changed.

## Go viewer invalid-frame and tile bounds (3ef11c8 all-five CI verified)

The Go viewer rejects undecodable full JPEGs and clears its old composition base. A failed delta also clears the base; subsequent deltas wait for a valid keyframe rather than mixing stale state. Tile rectangles must be positive, inside the base using overflow-safe comparisons and exactly match JPEG header dimensions before decoding/painting. This prevents unbounded rectangle loops and silent clipping/mismatched tile dimensions. It is not a general image-memory cap or a full untrusted-codec audit.

New stale-keyframe and invalid-tile regressions failed before fixing. Recovery, malformed-delta, nil-base, out-of-bounds/overflow and header/rectangle mismatch tests pass. Targeted race checks passed30 repeats; fresh session/mobile and full-repository race tests pass. All-five [3ef11c8 CI](https://github.com/MaisySen/Mnet/actions/runs/43929346383) passes. Three pre-existing transport tests had placeholder image bytes, now replaced with real synthetic JPEGs; the TCP case retains120000-byte payloads using trailing padding to test segmentation. These are local Go tests, not physical screen capture or GUI pixels. No fresh Android bindings, APK/AAB, release or live deployment were produced for this change; prior Java/dex checks apply only to their named earlier source.

## Recorder invalid-chain and close cleanup (3b959e2 all-five CI verified)

Session recording now clears the composition base after an empty/invalid keyframe or failed delta, matching the viewer's wait-for-valid-keyframe recovery. Close releases retained frame pixels, and late frame callbacks return without decoding or changing recorder state. Frame/close ownership remains under the recorder mutex.

Before-fix tests reproduced stale-base/delta recording and retained pixels after close. All recorder tests, including concurrent close/frame callbacks and valid-keyframe recovery, pass30 race-enabled repetitions. A fresh full-repository race test run also passes, and all-five [3b959e2 CI](https://github.com/MaisySen/Mnet/actions/runs/43931473995) is green. This is Go file/codec/concurrency evidence, not actual device recording or playback UI proof. No new Android bindings, APK/AAB, published release or live deployment.

## Current-source Android build refresh (3b959e2 source)

A fresh Android arm64 AAR from current Go3b959e2 source built with minimum API24. SHA256: `23cd2ba5860558715aa65cea64e0e951013dad65985d5aff30c6ae8b870b81c1`. Full Android34 resources/application Java and min-api24 dex compile against those bindings. The selected framework simulation reran64 cases successfully; the harness guide is consolidated into current coverage, explicit replacements and known gaps rather than stale pending extensions.

This supersedes the earlier current-binding build gap for3ef11c8/3b959e2. It is arm64 build evidence only, not all-ABI packaging, JNI execution, install/boot, valid projection, real input, actual Android UI pixels or device/emulator proof. The64 simulation cases still use fake Go/VPN/accessibility endpoints and synthetic frames. No APK/AAB was signed or published and the existing release/live deployment is unchanged.

## JPEG pixel-allocation policy (48fcffc all-five CI verified)

Go viewers and recorders check JPEG headers before full-image decoding. Each dimension must be positive and at most8192, and their product must be at most16,777,216 pixels. The division-based check avoids integer overflow.4K3840x2160 fits;8K7680x4320 does not. Tile headers and the composition base use the same dimension policy. An oversized keyframe is rejected and invalidates its old delta chain.

A tiny JPEG fixture with rewritten65535x65535 header dimensions tests refusal without allocating that pixel image. Boundary/overflow tests, viewer/recorder rejection and existing recovery tests pass30 race-enabled repetitions. A fresh full-repository race run and all-five [48fcffc CI](https://github.com/MaisySen/Mnet/actions/runs/43937696893) pass. This bounds a decoded image's pixel dimensions, not codec CPU, all decoder overhead, concurrent sessions or aggregate heap; compositing/callbacks can keep multiple images. No current-source Android binding rebuild, APK/AAB publication, device/capture/UI/JNI proof or live deployment for this change.

## Aggregate delta tile work (13fa2e2 retry verified)

A delta is limited to4096 tiles and at most the base frame's pixel count summed across all tile rectangles, before pixel decoding or composition. Regular nonoverlapping64x64 updates fit the budget; repeated overlapping rectangles that exceed one frame's work are refused. This adds a per-delta count/pixel-work limit to the existing individual geometry/JPEG checks, not a general CPU or total memory guarantee.

The aggregate-overlap regression failed before the guard. Tests accept exact four-quadrant coverage and reject excess aggregate pixels or tile count. Targeted frame/recorder/composition race checks pass30 repeats; a fresh full-repository race run passes. [13fa2e2 CI attempt2](https://github.com/MaisySen/Mnet/actions/runs/43939827797) passed after the first dependency-download failure. Synthetic Go codec data only, not physical capture/UI/JNI proof. No current-source Android AAR, release APK/AAB or live deployment produced for this change.

## Framework dependency rate-limit recovery (b581b89 all-five CI verified)

The first [13fa2e2 CI attempt](https://github.com/MaisySen/Mnet/actions/runs/43939827797) passed native Windows/macOS, kernel and session jobs, but Go ended during dependency resolution: Maven Central returned429 for Robolectric native runtime1.0.20 before framework tests ran. This is not an all-five pass. One failed-job retry passed on attempt2.

CI now enables setup-java's Maven dependency cache keyed by the pinned framework POM. It reduces repeated downloads without skipping tests or swallowing errors. The existing local cache ran all64 framework cases successfully. [b581b89 all-five CI](https://github.com/MaisySen/Mnet/actions/runs/43941974846) passed and its Go log confirms the Maven cache saved. Restore is not yet verified; cold-cache/network failures remain possible. No production source, artifact or live deployment changed.

## Four-ABI current-source Android binding build (b581b89 source)

Fresh current-source Go bindings built at minimum Android API24 for armeabi-v7a, arm64-v8a, x86 and x86_64. Archive inspection confirms all four native libraries; ELF inspection confirms their architecture classes. AAR SHA256: `0b5b52659366e345d338d9db2916f057e3b93fdc8fcbc878fefa0cd25a5f60a8`. Full Android34 resources/application Java and min-api24 dex compile against the new bindings.

This closes the binding-build gap after the JPEG/delta work changes and extends earlier arm64-only compilation. It is not installation, linker/loading on Android, JNI execution, hardware capture/input, UI pixels, boot or package-store validation. No APK/AAB signed or published and no live service change. Maven cache save is confirmed by b581b89 CI; cache restore needs a later run.

## Session truncated-write error detection (4bce9b8 all-five CI verified)

Session framing now returns io.ErrShortWrite if a writer reports fewer header or payload bytes without an error. It does not claim success or continue with a payload after an incomplete header; existing transport errors remain unchanged. The mux's existing write-error path closes the session. This protects framing at its io.Writer boundary, including custom adapters that violate the usual short-write error contract; it is not automatic retry or a reproduced kernel TCP failure.

A short-header regression failed before the fix. Header/payload truncation and transport-error preservation tests pass100 race-enabled repetitions; fresh full-repository race tests pass. All-five [4bce9b8 CI](https://github.com/MaisySen/Mnet/actions/runs/43946082516) passes. Synthetic writer evidence only, no live network/device/runtime claim. No new bindings, release APK/AAB or deployment. Separately, [905a401 all-five CI](https://github.com/MaisySen/Mnet/actions/runs/43944038163) confirms Maven cache restoration and64 passing framework cases.

## Session Dial transport ownership (2e87ce6 all-five CI verified)

Dial now explicitly owns its supplied connection: a failed hello write, reply read/parse, unexpected frame/version or refusal closes it. Success transfers ownership to the returned mux, which closes it later. Callers still supply handshake deadlines; this does not add a timeout or change consent rules. Existing caller-side error cleanup may close an already-closed connection, which net.Conn permits.

Counted net.Pipe regressions failed before the change for bad reply/frame/version/refusal and failed hello write. Error closure and successful mux ownership pass30 race repetitions; fresh full-repository race tests pass. All-five [2e87ce6 CI](https://github.com/MaisySen/Mnet/actions/runs/43948329722) passes. These are local synthetic transport tests, not a reproduced live descriptor leak or device/OS-network proof. No fresh bindings, release APK/AAB or deployment.

## CLI session handshake deadline (4f93413 all-five CI verified)

CLI session connect/reconnect and file-send now allow70 seconds for hello/host consent after TCP connects, rather than waiting indefinitely for a silent host. The existing TCP connect limit remains10 seconds. A successful handshake clears its deadline before active use; a failure closes the connection through Dial ownership. Expiry can terminate a delayed consent response and requires another attempt; it does not change authorization policy.

Short-budget silent-host and successful deadline-clear net.Pipe tests pass30 race repetitions. Fresh full-repository race tests pass. These use an accelerated fixture timeout, not an elapsed70-second real host/device test. All-five [4f93413 CI](https://github.com/MaisySen/Mnet/actions/runs/43950358961) passes. No mobile source, bindings, release artifacts or live deployment changed.

## Headless desktop host hello budget (d55f4d3 all-five CI verified)

The desktop session host bounds accepted-socket hello/reply I/O to15 seconds, then clears the deadline before starting capture. A silent peer no longer occupies the serial accept/hello loop indefinitely. Headless authorization remains immediate and unchanged; mobile's separate consent allowance is unchanged. This is a deadline, not a per-peer admission/connection-rate limit.

An accelerated40ms real OS-loopback test accepts a silent connection, verifies timeout, accepts a subsequent approved viewer and measures a successful keepalive round trip after the original handshake deadline.30 race repetitions and a fresh full-repository race run pass. No physical host, public-internet timing,15-second elapsed run, capture/input or device proof. All-five [d55f4d3 CI](https://github.com/MaisySen/Mnet/actions/runs/43952410300) passes. No bindings, release artifacts or live deployment changed.

## File receiver idle cleanup (5ff8e90 all-five CI verified)

Incoming file reception now allows60 seconds between data/FIN frames by default. A silent incomplete stream returns, closes/removes its private temporary file and releases its stream entry without closing unrelated session channels. FileReceiver.IdleTimeout can select a different positive budget; nonpositive uses the default. Active progress restarts the per-receive wait, so this is not a total-transfer deadline or resumable-transfer feature. A sender paused past the budget must retry the file.

A short40ms net.Pipe test sends a partial file, waits for idle cleanup and checks empty destination directory, stream release and a still-open mux. Idle/safe-file/reset tests pass30 race repetitions; a fresh full-repository race run passes. All-five [5ff8e90 CI](https://github.com/MaisySen/Mnet/actions/runs/43954473611) passes. No elapsed60-second real network test, device/runtime, new bindings, release APK/AAB or live deployment proof.

## File idle budget requires byte progress (061eecb all-five CI verified)

Only a successfully written nonempty data chunk restarts the incoming file idle budget. Empty data or unrelated frame types cannot retain a partial file indefinitely. A FIN still completes an empty offered file. Real chunk progress can continue beyond one budget duration, so this remains an idle timeout, not a total-transfer cap.

The empty-frame regression failed before fixing. Empty-frame refusal, partial cleanup and a progressing transfer lasting longer than its configured budget pass20 race repetitions, followed by fresh full-repository race tests. All-five [061eecb CI](https://github.com/MaisySen/Mnet/actions/runs/43956537225) passes. Accelerated net.Pipe fixtures only, not60-second real-network/device evidence. No new bindings, release artifacts or live deployment.

## Idle transfer stream-reset notification (b77bf55 all-five CI verified)

An incoming file idle expiry now sends a reset on that stream before releasing its local state. The sender sees ErrStreamReset instead of waiting for a later ACK timeout. Peer resets and mux closure do not trigger a reset echo. The reset uses the existing bounded outbound queue; delivery is not guaranteed when the connection is broken or blocked, and this does not add transport cancellation of already queued chunks.

A sender-notification regression failed before the change. Idle/reset/progress tests pass20 race repetitions, followed by fresh full-repository race tests. They also check partial cleanup, stream release and a still-open unrelated mux. These are accelerated net.Pipe fixtures, not60-second real-network or device proof. All five jobs passed in [b77bf55 CI](https://github.com/MaisySen/Mnet/actions/runs/43958595910). Fresh bindings are recorded below; no release artifacts or deployment.


## Current Android binding rebuild and full local gates (b77bf55)

Fresh bindings include the session framing, connection ownership and file-idle changes since the earlier b581b89 build. All four JNI libraries were extracted and their ELF architecture classes inspected: armeabi-v7a, arm64-v8a, x86 and x86_64. Minimum Android API is 24. AAR SHA256: `7b088f63c31519ee16161bfc98ad0b8f532849c2f4121824f7bfa8a1600d6827`. Full Android 34 resource generation and application Java compilation, plus min-api-24 dex compilation against this AAR, passed. A fresh framework run passed 60 service/helper and 4 Activity cases with no failures, errors or skips.

The full local gate bundle passed: Go build, Windows/macOS cross-builds, vet, vulnerability scan, fresh full-repository race tests, normal and burst-loss mesh-file stress, source-gate tests and secret scan. All five [b77bf55 CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43958595910) passed. This remains build/simulation/controlled-fixture evidence, not Android installation, JNI loading/execution, physical capture/input, actual Android UI pixels, boot or public-internet soak proof. No APK/AAB signed or published, no hosted deployment and no live device state changed.

Review at b77bf55 confirmed a remaining cleanup limit: idle-expiry reset enqueue uses the ordinary blocking outbound queue. If that queue is full because the peer stops reading, local partial-file cleanup can wait past the idle deadline. The current timeout tests do not exercise that backpressure case. That revision still needs a bounded/nonblocking termination path; the source fix is recorded in the next section.


## Idle cleanup without outbound queue waiting (b4600bc all-five CI verified)

A saturated-queue regression failed before this fix: incoming file idle expiry waited for reset enqueue, retaining the partial file and stream. The receiver now uses a best-effort reset helper that checks available queue capacity and skips notification if full, not started or closed. It never waits for capacity or exceeds the existing byte bound. Other send paths retain ordinary backpressure. No shared session close or queued sender-chunk cancellation is added.

The deterministic fixture uses a started mux without a writer loop, accepts a partial incoming file, then saturates outbound capacity. It checks cleanup, stream release, an unchanged full queue and an open mux. This is a queue-boundary fixture, not a stalled real-socket timing test. Existing available-capacity sender-reset notification, empty-frame refusal and progress tests plus this fixture pass 20 race repetitions. Fresh full-repository race tests pass. Filesystem cleanup/scheduling are not given a hard wall-clock guarantee. The last four-ABI Android build remains b77bf55, before this source change. No release artifacts or live deployment changed.


## Releasing a full inbound stream unblocks the read loop (fe877fa all-five CI verified)

A released dynamic stream now closes a separate local delivery-cancellation signal. The read loop can abandon an in-flight DATA or reset delivery to that stream's full queue and continue processing other streams. It remains the only owner that closes frame queues. Repeated local release is safe. Both locally opened and peer-opened production streams initialize the signal.

A net.Pipe regression fills one stream queue and observes a second frame blocked behind its delivery. Before fixing, releasing the first stream did not resume the second frame: both DATA and reset variants failed on the pipe write deadline. After fixing, both variants pass 50 race repetitions, with unrelated-stream payload delivery and an open mux checked. Fresh full-repository race tests pass. This is a controlled in-process pipe fixture with a short synchronization wait, not physical-network or device proof. It does not remove ordinary head-of-line blocking while an unreleased consumer is slow, discard already buffered frames or cancel outbound queued chunks. No current JNI/device validation, release artifact or hosted deployment is added; the last four-ABI binding build remains b77bf55.


## Release-signal constructors and current binding rebuild (fe877fa source)

A new lifecycle test opens streams through the actual Open/OnStream paths in both client-to-host and host-to-client directions. It checks that both local and peer stream states have live delivery-cancellation signals, repeated release closes each signal, and the shared mux remains open. Combined with the full-queue DATA/reset tests, 50 race repetitions pass. This checks construction/release, not end-to-end file transfer or device behavior.

The full local gate bundle passed again with these test additions: build, Windows/macOS cross-build, vet, vulnerability scan, uncached full race suite, normal/burst mesh-file stress, source-gate tests and secret scan. Fresh Android bindings at fe877fa include both queue-cleanup fixes: four ABI libraries extracted/ELF-inspected, Android 34 resources/all app Java and min-api-24 dex compile passed. AAR SHA256: `186647caaf80386e92c822b1d13602850baa689dbd21c57290e4248f0ba3ebc2`. A fresh framework run passed 64 cases with no failures, errors or skips. All five [b4600bc CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43961403091) and [fe877fa CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43963449817) passed; new constructor-test/documentation revision CI is not yet recorded.

No signed APK/AAB, publication, deployment, physical Android installation, JNI loading/execution, real capture/input, Android UI pixels or reboot proof is added. Build artifacts remain local validation outputs. The current source still permits head-of-line blocking for an unreleased slow stream and does not cancel queued outbound file chunks.


## Protocol-v1 unsupported frame header refusal (5b58ea5 all-five CI verified)

readFrame now rejects unknown message types, priorities outside the four defined classes and nonzero reserved header bits before allocating or reading payload bytes. All six known types and all four priorities remain accepted. Current writers already emit these supported headers with zero reserved bits. A future protocol extension must negotiate/update the format rather than rely on silently accepted v1 headers. No new message shape, peer identity or authorization is added.

Seven malformed-header regressions failed before fixing: they reached payload EOF rather than refusing the header. The tests advertise a 1MiB payload but supply no payload bytes. Known-type/priority round trips and a net.Pipe active-mux test verify supported framing and closure on a malformed header without waiting for payload. These checks pass 100 race repetitions; a fresh full-repository race run passes. This does not reject every malformed message body or replace the existing payload-size limit, peer authentication or rate limits. The latest all-five source remains fe877fa, c8bdaa4 has four successful jobs/macOS pending at this check, and the latest Android binding build remains fe877fa. No release, live deployment or physical runtime evidence changed.


## Recording destination no-clobber creation (c5bf4c8 all-five CI verified)

NewRecorder creates a new mode-0600 file with O_EXCL rather than truncating an existing path. An existing regular destination, final symlink or directory now fails without modification. This also avoids inheriting an older destination's broader file permissions. It is an intentional no-overwrite rule: callers must choose an unused path. The daemon already generates viewer/timestamp names and logs recording-open failure while continuing the session; a same-second collision now preserves the previous recording instead of replacing it.

Before-fix tests reproduced existing-file truncation and symlink-target truncation. Existing bytes/mode and final-symlink preservation, 12 concurrent creators with exactly one owner, and recorder codec/lifecycle tests pass 50 race repetitions. The symlink test can skip when the platform lacks symlink rights; it executed locally. Fresh full-repository race tests pass. O_EXCL protects the final destination, not attacker-controlled parent-directory substitution or recording write failures/disk exhaustion. This is local file/concurrency evidence, not physical playback/capture or a live-file repair. The latest all-five source is c8bdaa4; header-refusal5b58ea5 has four successful jobs/macOS pending at this check. Current bindings remain fe877fa. No release artifact, deployment or existing user recording changed.


## Recording output failure retention (e110af8 all-five CI verified)

A recorder now stops on its first output error, closes the writer, releases its pixel base and ignores late frames. It counts only complete successful frame writes. A short write without an error is converted to io.ErrShortWrite. Close preserves the first write error across repeated calls; if writing succeeded, a close error is retained instead. The daemon checks Close and logs recording incomplete at session end before its existing path/frame-count log. Hosting is not ended, partial recording bytes are not deleted and no automatic retry is attempted.

A read-only-file-handle regression failed before fixing because the recorder retained writer/pixels and lost the write error at Close. It now verifies zero recorded frames, released resources, unchanged fixture bytes, repeated-close error retention and ignored late callbacks. This is deterministic invalid-handle failure injection, not actual disk exhaustion, fsync/durability or partial-write filesystem proof. Recorder/create/concurrency checks pass 50 race repetitions; fresh full-repository race tests pass. Error reporting remains at session end, not an immediate UI alert. Bindings still name fe877fa, before this work. No release or live deployment changed.

Prior source verification: all five [5b58ea5 CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43967596216) and [c5bf4c8 CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43969548449) passed.


## Recorder error branches and current four-ABI build (e110af8 source)

Additional tests inject a failed output handle after a valid keyframe, then attempt a delta: the first completed frame remains counted, the failed delta is not counted, resources are released and Close reports the write failure. A separately preclosed output handle checks close-error retention over repeated Close calls. Recorder/create/concurrency checks pass 50 race repetitions. These are controlled invalid-handle tests, not disk-full, silent-short-write or fsync/durability proof.

The full local gate bundle passed again with the branch-test additions: build, Windows/macOS cross-build, vet, vulnerability scan, uncached full race suite, normal/burst mesh-file stress, source-gate tests and secret scan. Fresh e110af8 bindings include header refusal and recording changes for armeabi-v7a, arm64-v8a, x86 and x86_64 at minimum API 24. Archive/ELF inspection and Android 34 resources/all app Java/min-api-24 dex compilation passed. AAR SHA256: `c367be1a63128ced90995debf2b275eeadbc7f70f96aafd2cc224a61e5efa5f8`. Fresh framework simulation passed 64 cases, zero failures/errors/skips. All five [e110af8 CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43971473662) passed; the additional-test/documentation revision has not yet completed CI.

No APK/AAB signed or published, hosted deployment or live device state changed. Build/simulation is not installation, JNI loading/execution, physical capture/input, Android UI pixels or reboot proof. No real recording file was repaired or altered by these tests.


## Malformed viewer screen JSON breaks the delta chain (ded8d94 all-five CI verified)

The viewer screen loop now clears its stored frame base when screen-message JSON cannot be decoded. Previously, it skipped that message before resolveFrame ran, leaving earlier pixels available to a later delta. A valid keyframe is required before delta playback resumes. Valid known frame behavior and callbacks remain unchanged.

An actual mux/viewer-loop net.Pipe regression sends a valid keyframe, malformed JSON, a delta and a recovery keyframe in wire order. It failed before fixing by delivering the stale-base delta. It now observes the recovery keyframe as the next callback. Viewer tests pass 50 race repetitions, followed by a fresh full-repository race run. Synthetic JPEG/wire fixtures only, not real capture/display/UI proof. This does not add sequence-gap detection, reject every semantic JSON ambiguity or change the host recorder's typed-frame path. All-five [e291bd8 CI](https://github.com/MaisySen/Mnet/actions/runs/43973623419) is verified; fresh bindings still name e110af8, before this viewer source change. No release or deployment changed.


## Viewer screen-loop exit releases decoded pixels (351be62 all-five CI verified)

The viewer screen loop clears its decoded pixel base under the existing frame mutex when it exits. A reachable Viewer no longer retains that base after its loop observes FIN, peer reset or mux closure. This cleanup does not erase JPEG bytes or image copies an application callback retained, clear the geometry descriptor or interrupt a callback that blocks instead of returning. It is loop-exit cleanup, not a new Close API or hard memory-lifetime guarantee.

Direct screen-loop fixtures for FIN/reset/closed mux all failed before fixing by retaining pixels. The fixtures now observe loop exit and an empty base. Viewer checks pass 50 race repetitions, followed by fresh full-repository race tests. These tests use a synthetic pixel base and controlled channel/pipe state, not Android/desktop display or real-device memory measurements. All-five [ded8d94 CI](https://github.com/MaisySen/Mnet/actions/runs/43975421785) is verified. Current four-ABI binding build still names e110af8, before the two viewer source changes. No release, deployment or user-device state changed.


## Current viewer bindings and session guide consolidation (351be62 source)

Fresh Android bindings include malformed-screen-JSON recovery and screen-loop pixel cleanup on all four ABIs at minimum API 24. Archive/ELF inspection confirms armeabi-v7a, arm64-v8a, x86 and x86_64. Android 34 resources/all app Java/min-api-24 dex compilation passed. AAR SHA256: `63683e07786321da988f109f11eabfd160eafb3155d79cf1d6291d8d569bf7a5`. A fresh framework run passed 64 cases, zero failures/errors/skips. The full local gate bundle passed in a separate complete rerun; an earlier combined build/gates command hit its execution timeout during stress checks and is not counted as a pass. All five [351be62 CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43977271776) passed.

The owning session-layer guide now groups current image, handshake, file-idle/reset, stream-release and recording behavior rather than repeating stale chronological pending claims. Exact old measurements remain here as history; STATUS and the newest versioned checks supersede them. No signed APK/AAB, release, deployment or user-device state changed. Build/simulation still does not prove JNI/device/physical capture/input/UI/boot behavior.


## Host capture failure ends the session transport (0396795 all-five CI verified)

The host screen loop closes its mux when screen production exits, including ordinary capture errors, delta-capture errors and periodic-keyframe fallback errors. Previously those returns left keepalives running and a viewer connected to a frozen screen. Transport closure ends this session's chat/file traffic too. It does not invoke Stop from the screen worker, which would wait on itself; the existing owner still calls Stop for capture cleanup and worker waits. Recording-only output errors remain nonfatal to hosting.

All three net.Pipe host/viewer failure regressions failed before fixing by leaving the viewer connected. They now observe both transport closures, pass 50 race repetitions and are followed by a fresh full-repository race pass. Capturers are explicit failure fakes; no physical capture failure, device recovery, capture-timeout or reconnect proof is implied. A capture call that blocks forever is not interrupted by this change. All-five [22ed76c CI](https://github.com/MaisySen/Mnet/actions/runs/43979463842) is verified; current bindings remain 351be62, before this host source change. No release, deployment or user-device state changed.


## Capture-failure owner cleanup and current bindings (0396795 source)

A new mobile-owner test starts the actual listener/session workflow on OS loopback with explicit callbacks whose capture fails. Two successive viewers each disconnect, receive exactly one session-end callback, clear active state and leave the same listener able to accept the next viewer. It passes 20 race repetitions. This checks Go owner/callback/listener reuse, not Android MediaProjection, JNI, physical capture, UI or reboot.

The full local gate bundle passed with this test addition: build, Windows/macOS cross-builds, vet, vulnerability scan, fresh full race suite, normal/burst mesh-file stress, source-gate tests and secret scan. Fresh 0396795 Android bindings include host capture-failure shutdown on all four ABIs at minimum API 24. Archive/ELF inspection and Android 34 resources/all app Java/min-api-24 dex compilation passed. AAR SHA256: `25ee3d878ff2b37c068c8f80f70c0762ef1ad4a30cd1183a256bac07416b3b80`. Fresh framework simulation passed 64 cases, zero failures/errors/skips. All five [0396795 CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43981179826) passed; the owner-test/documentation revision is not yet CI verified.

No signed/published APK/AAB, deployment or existing user-device state changed. Physical-platform/JNI/capture/input/Android UI/boot gaps remain unchanged.


## CLI saved-frame private replacement (a1f503c all-five CI verified)

The CLI's intentional latest.jpg replacement now uses a mode-0600 same-directory temporary file, checks the write/close result, then renames over the destination. It does not open/truncate or follow the destination's final symlink. New save directories request 0700 rather than 0755. Existing parent directory permissions are not changed; parent paths must be trusted. Windows permission/rename semantics remain platform-specific, not Unix privacy/atomicity certification.

Before-fix tests reproduced mode0644 snapshots and symlink-target overwriting. Private-mode checks (non-Windows), repeated latest-frame replacement, target preservation and no temporary-file leak pass 50 race repetitions. Symlink tests can skip when platform rights are unavailable; executed locally. Fresh full-repository race tests pass. Fixtures use plain byte strings to test filesystem handling, not actual display pixels. This does not guarantee durability, harden parent-directory substitution, alter older snapshots elsewhere or show write errors in an immediate UI. All-five [c7dd7b3 CI](https://github.com/MaisySen/Mnet/actions/runs/43983110424) is verified. Shared Go/mobile source and its 0396795 bindings are unchanged. No release, deployment or existing user file changed.


## Snapshot replacement failure and permission regression coverage

Additional tests refuse replacement of a nonempty destination directory, preserve its sentinel bytes and verify no same-directory temporary file is left. Replacing an older0644 regular snapshot creates a new0600 file on non-Windows platforms rather than inheriting broad mode bits. All snapshot filesystem tests pass 50 race repetitions. Full local gates pass with these additions: build, Windows/macOS cross-builds, vet, vulnerability scan, uncached full race suite, normal/burst mesh-file stress, source-gate tests and secret scan.

All five [a1f503c CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43984790512) passed. Its native Windows/macOS jobs execute the CLI tests, with symlink/Unix-mode checks conditional on platform capability. The additional failure-path revision is not yet CI verified. Shared Go/mobile source and the0396795 binding build are unchanged. No new physical capture/UI/privacy-ACL certification, signed release, deployment or existing user snapshot mutation is implied.


## CLI session connect option validation (new source, CI pending)

The CLI now parses and validates connect options before entering the connection workflow. Previously unknown/trailing tokens and missing values were ignored, repeated flags silently selected a later value and a following flag could become an earlier flag's path. Only the two documented optional flags are accepted once each; missing/empty/flag-like values reject. Flag-like paths remain usable with a ./ prefix. No filesystem/network work occurs inside this parser.

Ten malformed argument fixtures failed before fixing. Rejection, both valid option orders, exact preservation of space-containing/flag-like paths and empty-options compatibility pass 100 race repetitions. Fresh full-repository race tests pass. These are parser unit tests, not an interactive shell or real-peer invocation. Shared/mobile source and current0396795 bindings are unchanged. All-five [b4c78e6 CI](https://github.com/MaisySen/Mnet/actions/runs/43986500014) is verified. No release, deployment or user file/device state changed.


## Dirty-tile canvas-size change handling (556399f all-five CI verified)

DiffTiles now treats different prior/current bounds as a full refresh. Previously image growth could index past the old pixel buffer; shrink could report no changed tiles for an incompatible canvas. Both X11 and portal capture callers clear their prior delta base when bounds differ, ensuring full-frame encoding rather than minority-tile selection. Android capture/input policy is unchanged.

A synthetic8x8-to16x16 regression panicked before fixing with an out-of-range slice. Growth and shrink now produce the same tile list as a fresh image. DiffTiles checks pass 100 race repetitions, followed by fresh full-repository race tests. Caller changes compile in the local build/test surface; no actual desktop display resize, portal renegotiation, physical output pixels or input geometry behavior was exercised. This does not validate arbitrary tile sizes, nonzero-origin images or all malformed image buffers. All-five [f3c9d87 CI](https://github.com/MaisySen/Mnet/actions/runs/43988086926) is verified. Current Android bindings still name0396795 before this shared Go helper change. No release, deployment or existing user/device state changed.


## Portal canvas fixture and current bindings (556399f source)

An added portal-capturer fixture seeds smaller/larger prior image bounds, feeds one fixed-size raw frame and checks full JPEG output at the new dimensions with no delta tiles. Portal capturer tests pass 50 race repetitions. It runs the actual capturer selection/encoding code but bypasses D-Bus/GStreamer with a raw pump. This is not live display/stream renegotiation or X11 runtime proof.

The full local gate bundle passed with that fixture. Fresh 556399f Android bindings cover all four ABIs at minimum API 24. Archive/ELF inspection and Android 34 resource/all app Java/min-api-24 dex compile passed. AAR SHA256: `b4e4c3fb859567f4eb5dc9e396ce4edb455d82c7430af155a423f914175cc009`. Fresh framework simulation passed 64 cases, no failures/errors/skips. All five [556399f CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43989677724) passed; the new fixture/documentation revision is not yet CI verified. No signed/published APK/AAB, deployment, physical JNI/capture/input/UI/boot proof or user-device state change is added.


## Tile encoder rejects clipped geometry (6b2fe37 all-five CI verified)

EncodeTiles validates its full rectangle list before output allocation/encoding. Nil sources and empty/negative/outside/overflow-position rectangles reject, rather than relying on SubImage clipping. Valid rectangles produce JPEG dimensions matching their advertised geometry. This is an encoder boundary check; generated desktop tile requests remain valid.

Seven invalid rectangle regressions were accepted before fixing. Rejection, nil-source handling, a valid nonzero-origin source/rectangle and existing diff/encode tests pass 100 race repetitions. A fresh full-repository race suite passes. This does not validate malformed image.Pix/Stride buffers, bound aggregate encoding CPU or add nonzero-origin support to DiffTiles. All-five [ea9ff62 CI](https://github.com/MaisySen/Mnet/actions/runs/43991510739) is verified; latest bindings remain556399f, before this shared helper change. No release, deployment or existing user/device state changed.


## Composite base stride preservation (d0afcb6 all-five CI verified)

CompositeTiles now copies base pixels row by row using PixOffset rather than a flat slice copy. Valid RGBA subimages can have a larger parent stride, so the previous copy could mix neighbor/padding pixels into subsequent rows. Output retains the base's bounds but does not alias its pixel buffer.

A gradient subimage fixture failed before fixing at its second row. It now compares every output/base pixel and verifies output mutation does not alter the base. Composite/encode/diff checks pass 100 race repetitions, followed by fresh full-repository race tests. Synthetic image-layout evidence only; production decoded bases are normally tightly packed. This does not validate arbitrary malformed buffers, add DiffTiles offset-origin support or prove real display pixels. All-five [6b2fe37 CI](https://github.com/MaisySen/Mnet/actions/runs/43993052318) is verified; current bindings remain556399f before encoder/compositor changes. No release, deployment or user/device state changed.


## Current encoder/compositor bindings and validation (d0afcb6 source)

Fresh bindings include encoder rectangle refusal and stride-aware base copying across all four Android ABIs at minimum API 24. Archive/ELF inspection and Android 34 resources/all app Java/min-api-24 dex compilation passed. AAR SHA256: `63a069a60fb0cfb9b29f1e3d875a6cc4f79bd5c906bc4ddd689499822aff7645`. Fresh framework simulation passed 64 cases with zero failures/errors/skips. Full local gates passed: build, Windows/macOS cross-builds, vet, vulnerability scan, fresh full race suite, normal/burst mesh-file stress, source-gate tests and secret scan. All five [d0afcb6 CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43994622318) passed.

STATUS, the framework guide and current session-layer behavior now name this validation source. The following documentation revision has not yet completed CI. No signed/published APK/AAB, hosted deployment or existing user-device state changed. These build/simulation/controlled-fixture results do not execute JNI, physical capture/input, Android UI pixels, installation or reboot.


## Android API 29+ always-on disconnect controls (new source, CI pending)

VpnAlwaysOn guards the system isAlwaysOn query behind API 29. A true mode hides the service notification's disconnect action, includes the mode in private status broadcasts, disables the connected main-screen button and defensively ignores stale disconnect intents. Revoke/destroy/failure cleanup is untouched. No system setting is changed. API 24-28 detection remains unknown and earlier controls remain; this partial source change does not declare boot/always-on support.

Fourteen API/mode combinations pass against JVM fakes that throw if the API 29 method is called on an older SDK. Source-wiring assertions cover the service/notification/activity conditions; these are not execution or pixel proof of those paths. Existing API 24-36 compatibility fakes pass. Full Android 34 resources/all application Java/min-api-24 dex compile passes against current d0afcb6 bindings; shared Go code did not change. Existing 64 framework cases pass, but their VPN endpoint is fake and MainActivity is not included. Actual mode changes, Settings interaction, notification/main-screen pixels, JNI, device VPN/reconnect and reboot remain unverified. A new CI step runs the JVM-fake policy/wiring check.

Sources checked: [Android VPN guide](https://developer.android.com/develop/connectivity/vpn), [VpnService reference](https://developer.android.com/reference/android/net/VpnService). All-five [6956dd5 CI](https://github.com/MaisySen/Mnet/actions/runs/43996353379) is verified. No signed release, deployment or existing user-device state changed.


## Always-on main-screen execution follow-up (new source, CI pending)

The framework suite now executes actual MainActivity using placeholder resources, fake VPN/HostingActivity endpoints and direct status-receiver calls on API24/25/26/33. Twelve new cases cover up/joining/waiting-approval controls, explanation text, direct stale-handler rejection, switching back to manual disconnect and down-state Connect availability. All76cases pass with zero failures/errors/skips. Removing disabled-button wiring or independently removing stale-handler rejection each fails four cases. Restored production code retains both guards. MnetVpnService now snapshots the volatile node before reading status on a managed stale disconnect, avoiding a double-read race with cleanup.

Main-screen application code executes in simulation, but placeholder resources do not validate actual strings/layout/pixels. System always-on detection, Settings transitions, actual VPN notification/disconnect branches, JNI, real receiver security and device reboot remain unverified. API24-28 legacy detection is still open. [5f0e92b CI](https://github.com/MaisySen/Mnet/actions/runs/43998045061) passed all five jobs before this follow-up. No release/deployment/live/device state changed.


## Always-on VPN service execution follow-up (new tests, CI pending)

A separate framework mode compiles actual MnetVpnService/VpnAlwaysOn/VpnPlatform with a fake Go node, placeholder resources and a shadowed system isAlwaysOn query. Four cases on each API24/25/26/29/33/34 produce24passes: manual notification action and stop cleanup; managed action omission and stale stop refusal with status extra; managed no-node request; and system revoke cleanup. Removing both managed notification/service conditions causes six failures. Restored source passes24 and the existing service/activity suite passes76, totaling100 with zero failures/errors/skips. CI runs both suites.

This executes notification objects and selected service branches, not notification pixels, real system mode detection, Settings transitions, VPN interface/worker, JNI, receiver security, physical reconnect or reboot. Legacy API24-28 detection remains open. Production source is unchanged from ec8450a; all five [ec8450a CI jobs](https://github.com/MaisySen/Mnet/actions/runs/43999580991) are verified. No signed release/deployment/live/device state changed.


## VPN worker startup/stop sequencing (CI passed)

Actual MnetVpnService now runs worker fixtures against a factory-controlled fake Go node. Missing saved config fails before node creation; prepare failure closes/reset ownership and permits retry. A latched prepare checks active duplicate start, manual stop, refusal while the canceled worker still unwinds, no canceled tunnel start and acceptance after exit. Latches release in finally; bounded joins pump Robolectric's main looper. An initial harness run blocked that looper and timed out, so it is not counted as a pass. Corrected fresh VPN suite passes36cases across API24/25/26/29/33/34, adding12 to the existing24. Existing selected service/activity suite passes76, total112. No production source changed.

Go/system mode and resources remain fake; this executes selected worker branches, not real VPN interface setup/JNI, all cancellation races, Settings transitions or physical reconnect/reboot. [3fdc248 CI](https://github.com/MaisySen/Mnet/actions/runs/44001182536) passed all five jobs before this follow-up. No released/deployed/live/device state changed.


## Android 0.8.3-dev private candidate build

The owner requested an installable test APK. Android manifest advances to code15 / version0.8.3-dev, separate from published0.8.2/code14. Built from b09858d plus that manifest-only version change, using d0afcb6 four-ABI bindings whose shared Go sources match current source. Existing Mnet upload signer SHA256: 61cfc5c891a2f8600981ece8f111e2b02c0e34ea355263f153d329373d000c6f signs APK v2/v3. Package/minSDK24/target34/launchable MainActivity, all four ABIs, ZIP integrity and alignment checks pass. APK SHA256: 22a20e5e696321eb4994c8d8e4886fc39f5888e7c66aad79744a28803e8a8158. No physical installation or Mnet phone behavior tested, and no hosted deployment/public release changed. Restricted delivery remains pending. All five [b09858d jobs](https://github.com/MaisySen/Mnet/actions/runs/44003438085) passed before this manifest/docs follow-up.
