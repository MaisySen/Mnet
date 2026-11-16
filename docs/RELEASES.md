# Releases

Test releases and engineering history. Historical entries record what was claimed at the time, not a current readiness guarantee. Older builds have known security/reliability gaps. Use [STATUS](STATUS.md) for source versus shipped/deployed state.

## Unreleased after v0.8.2-test.1

- Source-only subnet serving safety change: replace broad unowned NAT/global-forwarding mutation with unique destination-scoped Linux NAT ownership and an explicit operator-enabled forwarding prerequisite. Setup errors fail Up; unsupported automatic exit/mobile serving is rejected. Local tests and three fresh separate-LAN/control-off TCP fixtures passed in [6735500 CI](https://github.com/MaisySen/Mnet/actions/runs/43770289687). The disabled-forwarding failure path passed three fresh [fcbddd1 fixtures](https://github.com/MaisySen/Mnet/actions/runs/43771745649), including retry after the fixture enables forwarding. Older broad NAT rules are not adopted or removed.

- Bounded Linux RFC1918 subnet-route ownership implementation in progress: tagged add-only routes, conflict refusal, failed-batch rollback and scoped deletion. Unit regressions and actual kernel subnet/route-preservation extension pass in [8fcaf91 CI](https://github.com/MaisySen/Mnet/actions/runs/43764310572). Route-only ACL withdrawal/restoration passed once in [7a72378 CI](https://github.com/MaisySen/Mnet/actions/runs/43765437995); three fresh full-kernel runs passed in [2d23b6a CI](https://github.com/MaisySen/Mnet/actions/runs/43765976816). Admin expiry/reapproval with route teardown and corrected local up status passed three fresh [e6ab28d fixtures](https://github.com/MaisySen/Mnet/actions/runs/43767516777). Invalid additions no longer prevent withdrawal cleanup, and route counts/command duration are bounded. Exit routing remains separate and incomplete.

- Local enforcement of received self/peer expiry timestamps, independent of blocked control polls. WireGuard IPC, renewal/stale-map protection and repeated lifecycle regressions pass on Linux and native Windows/macOS. This cannot learn an unseen offline admin revocation.
- Direct-path fixes: healthy relay handshakes no longer suppress bounded direct retry; the node's own overlay IP is not advertised as an underlay candidate.
- Native Windows SCM install/start/stop/restart/crash recovery/remove passes on a disposable runner with an offline fixture. Wintun remains unverified.
- Linux kernel namespace fixture passes actual TUN/address/ping, direct UDP verification, then OS TCP transfers with the control/relay process stopped: 3 x 4MiB normal and 3 x 4MiB with 1% loss/20ms delay, receiver hash and sender ACK. Actual drops and Down interface/mesh-route removal checked. [Exact c4a1bcd CI](https://github.com/MaisySen/Mnet/actions/runs/43760903768).
- Documentation reorganized around status, operational limits and evidence. Exit/subnet OS routing is not declared working; subnet forwarding now has bounded Linux kernel proof; exit routing remains unsupported.
- These source changes are not in the v0.8.2 artifacts or the last verified hosted deployment.

A later local regression reproduced stale last_error after a successful retry. New source clears errors when a fresh Up attempt starts, preserving errors raised during that attempt. The stronger kernel retry assertion and native regression passed all five jobs and three fresh [4127209 fixtures](https://github.com/MaisySen/Mnet/actions/runs/43772484156), including 140/186/158 later loss-profile drops.

New source adds `mnet up --reset-routing` (while down) to clear saved serving/exit roles without deleting identity. Ordinary saved-state Up keeps its previous routing configuration. Reset cannot be combined with new routing roles. A local API regression reproduced the missing recovery path before this change; local API tests pass, including 20 race-detector repetitions. A bounded 30-second startup reachability assertion and explicit saved-identity comparison passed all five jobs and three fresh [85c2f79 fixtures](https://github.com/MaisySen/Mnet/actions/runs/43774062502), including 123/157/150 observed later loss-profile drops. This is bounded startup convergence, not arbitrary network readiness.

## Mobile preparation lifecycle (new source)

A fresh regression found that mobile Prepare still reported Down while enrollment was running, allowing other lifecycle operations to overlap. New source serializes Prepare with Up/Down, reports preparing, publishes initial and joined identity under the status lock, and returns independent routing slices from State/SetRouting. Prepare/API regressions passed 20 local race-detector repetitions and the full race suite passed locally. All five jobs passed in [718f74e CI](https://github.com/MaisySen/Mnet/actions/runs/43776983336), including native Windows/macOS preparation regressions and three full kernel fixtures with 152/152/152 later loss-profile drops. This is not physical mobile or automatic boot proof.

Preparation cancellation follow-up: the first serialization regression deliberately released a blocked join before Down could finish. A new regression showed Down could instead wait for the full preparation timeout. Prepare now publishes its cancellation function while entering preparing, so Down cancels blocked enrollment before joining the lifecycle lock. Blocked-Prepare cancellation and identity/status regressions passed 20 local race-detector repetitions; the full race suite/build/vet pass. All five jobs passed in [973a7e1 CI](https://github.com/MaisySen/Mnet/actions/runs/43777723099), including native cancellation regressions and three full kernel fixtures with 152/132/152 later loss-profile drops.


## Three-device kernel mesh

A new fixture adds a third independently keyed kernel VPN device to the same network, requires unique keys/IPs, two visible peers per device, all six directed mesh pings and third-device OS TCP 3 x 4MiB hash/ACK. It then removes that device and requires the original direct mesh to recover before the existing ACL/reapproval/control-off tests. All five jobs and three fresh fixtures passed in [799f3a1 CI](https://github.com/MaisySen/Mnet/actions/runs/43778473220), with 130/150/164 later loss-profile drops. Third-device transfers used an available control service; only later original-pair/LAN transfers were control-off. This does not implement a single-account ownership/login model or establish fleet capacity.


## Cross-network kernel isolation

A fresh identity joins a second network on the same disposable control service. The new fixture requires no original peers on that device, no outsider in the original pair maps, and failed access to distinct original mesh IPs. Independent network address spaces can overlap, so the isolated device's own IP is not treated as a cross-network target. The first run passed those assertions but failed a later reapproval helper in [b650b3e CI](https://github.com/MaisySen/Mnet/actions/runs/43779251604). The extended fixture repeatedly logged in instead of reusing its session; the observed failure is consistent with its login burst exceeding the existing limiter. The fixture now reuses its private temporary admin cookie rather than weakening the production limit. All five jobs and three complete fixtures then passed in [e0a8f84 CI](https://github.com/MaisySen/Mnet/actions/runs/43780009817), including the three-device mesh, cross-network isolation and later control-off profiles with 135/140/168 drops. Network-scoped transport isolation is not account ownership, organization membership or a multi-tenant security certification.


## OIDC approval provenance

Verified OIDC approval now stores issuer and subject separately from the display email in private control state. No principal is inferred for historical email-only approvals. Identity provenance persists atomically with approval, rolls back on save failure and is removed with the device. It is not added to peer-facing node maps. Local persistence, legacy-unknown, issuer-distinction, non-disclosure and failed-write regressions pass. This is account groundwork, not account ownership, membership, authorization based on email, a login dashboard or automatic device grouping. All five jobs passed in [c2fe64c CI](https://github.com/MaisySen/Mnet/actions/runs/43780929040), including three complete kernel fixtures with 131/144/138 later loss-profile drops. A further signed-provider callback regression now asserts the private issuer/subject was recorded and excluded from the returned map; the extra assertion passed all five jobs in [471f537 CI](https://github.com/MaisySen/Mnet/actions/runs/43781793743), with 149/143/176 later kernel loss-profile drops.


### Process startup history

A newer kernel extension gracefully stops the serving daemon process and starts a fresh process with saved state and `-up`, asserting unchanged identity and restored direct VPN/LAN. The first fresh-process restart passed, but the later one-shot LAN ping after ACL restoration failed in [b4046ed CI](https://github.com/MaisySen/Mnet/actions/runs/43774744281). Diagnostics also exposed a candidate inside an advertised subnet, which can route the tunnel endpoint back into itself. New source filters mesh/advertised-prefix endpoint candidates and uses bounded LAN convergence after route restoration; the kernel job passed three fresh full fixtures in [c33c6c5 CI](https://github.com/MaisySen/Mnet/actions/runs/43775496938), including fresh-process saved-state startup and 141/129/123 later loss-profile drops. The overall run was not green: the Go job failed one-second local API readiness and safe-file receive timeouts during the parallel race suite. Test-only readiness/receive budgets are now ten seconds; protocol/content/cleanup assertions and production timeouts are unchanged. All five jobs and three fresh full fixtures passed in [64eb696 CI](https://github.com/MaisySen/Mnet/actions/runs/43776218766), including 124/123/152 later loss-profile drops. The bounded process-startup batch is independently green. This is not a real reboot, systemd/launchd boot or forced-crash cleanup proof.


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

## v0.8.2-test.1 - 2026-11-16

Source `2bb55f4be60969a855b37fbb6d607f0831542e45`, Go 1.26.11, Android 0.8.2/code 14. [Release and 16 assets](https://github.com/MaisySen/Mnet/releases/tag/v0.8.2-test.1), [exact-source CI](https://github.com/MaisySen/Mnet/actions/runs/43756253912).

- Bounded relay queue drain wait and writer-side reauthorization; WireGuard counter parsing corrected against actual IPC.
- Proof isolation/diagnostics and sender ACK plus receiver hash required. Small attributed proof-only classic-SACK adapter; no production gVisor fork or timer-root-cause claim.
- Native Windows protected-DACL state/token protection, linked-path rejection and synced private replacement. Native Windows/macOS state/local-API/loopback-session checks passed.
- Android AAR regenerated before APK/AAB, existing upload certificate retained; upload hashes and anonymous APK/checksum download bytes verified at publication. Source/native/Android-arm64 vulnerability scans and known-secret artifact scan passed at build time, not a security certification.
- Same test service last verified at this source at 07:12 AEDT. Test-only; no important files, physical-platform or long-session guarantee.

## v0.8.1-test.1 and v0.8.0-test.1 - superseded test builds

[0.8.1](https://github.com/MaisySen/Mnet/releases/tag/v0.8.1-test.1) and [0.8.0](https://github.com/MaisySen/Mnet/releases/tag/v0.8.0-test.1) precede the consolidated 0.8.2 relay/storage updates. Their published warnings remain relevant to those versions. Do not use GitHub's older non-prerelease "latest" badge as the recommended test download.

## Development history through v0.8.2 (historical notes)

The following chronological engineering notes were previously grouped under a stale "0.7.1 draft" heading. They are preserved as history, not an unreleased inventory. Version-specific statements such as "newer than 0.8.0" or "Windows compile-only" describe that earlier checkpoint; the versioned entries above and STATUS supersede them.

Live proofs and the fixes they exposed.

- Local daemon controls now require a state-directory capability, including
  status reads; browser-origin requests are rejected. The CLI supplies it
  only to the local endpoint and refuses redirects. Malformed/oversized
  controls fail before launch. New CLI/daemon must be updated together;
  this source follow-up is newer than 0.8.0-test.1 downloads. Windows
  state-directory ACL and live-machine validation are still required.

- Relay forwarding is network-scoped and rechecks approval, expiry and
  pair-level ACL on every packet; expiration detaches current sockets.
  Expired nodes disappear from new peer maps. Updated daemons now remove
  revoked WireGuard peers, clear peers on unauthorized control responses,
  and prevent path refreshes from reinstalling an older map. Both client
  and server updates are required for the corresponding direct/relay
  protections; offline/outdated direct clients are not instantly revoked.

- Persistence failures roll back changed live state and enrollment-key
  use. Ten operation-level negative fixtures reproduced old partial-state
  behavior. SSO identity/approval persist together. State saves use private
  unpredictable temp files, file sync before rename, and failure cleanup;
  tests cover old predictable-temp symlink safety and rename failures.

- Security: authenticated rejoin now requires the saved node credential;
  the public WireGuard key can no longer recover a known identity token.
  Expired nodes no longer retain approval merely by rejoining. Current
  daemons supply saved credentials, and fresh enrollment authorization or
  admin reapproval is required after expiration. All previously published
  releases precede this fix and must not be treated as production-safe.

- Control listener has bounded header/body reads, writes and idle time,
  plus a smaller header limit. Map polls retain time to finish; a real TCP
  WebSocket echo test verifies upgrade traffic is not cut off by HTTP
  deadlines. Slow-header/body and oversized-header tests cover rejection.

- OIDC transport: bounded provider requests, no redirect following (a
  reproduced 307 credential-forwarding regression), exact discovery issuer,
  TLS endpoint policy with loopback-only HTTP for local tests, no upstream
  error-body echo. Authorization endpoints retain existing query values.

- SSO is deny-by-default: configuring an issuer also requires exact subject
  IDs or verified-email domains. Unlisted identities never approve nodes.
  Per-login nonce binding rejects tokens from a different login. Signed
  and end-to-end rejection tests cover these cases. Existing SSO operators
  must add an explicit policy; this is an intentional safe-default change.

- OIDC verification now requires exact issuer, subject, expiry and issued-at
  claims; rejects additional/untyped audiences and a mismatched authorized
  party. Signed negative fixtures reproduced the former acceptance gaps.
  Fake-provider end-to-end tests remain distinct from live issuer testing;
  admission policy is handled by a separate follow-up change.

- Approval and OIDC callback URLs now use the validated configured base
  origin, not caller-controlled Host or forwarding-protocol headers.
  Signed identity strings are HTML-escaped on the success page. Origin
  spoof and markup regressions cover both paths.

- Control ingress: ignore arbitrary forwarding headers by default; explicit
  trusted-proxy CIDRs enable right-to-left source resolution for both rate
  limits and audit events. Header-spoof bypass is covered by a regression
  that failed before the fix. Bound each IP table and amortize cleanup.
  Operators behind proxies must configure verified proxy ranges.

- Source gate: replaced shell-specific dash matching and broad directory
  exclusions with a portable tracked-source scanner. High-signal private
  key, provider credential and Mnet-token patterns include docs/tests/
  workflows; failures print only locations/types, not credential values.
  Positive/negative fixtures run locally and in CI. This is a narrow
  safeguard, not a replacement for history/artifact audits or Gitleaks.

- Daemon/API readiness: local API binding errors now fail the daemon
  instead of logging while it looks ready. The HTTP listener is context
  scoped and closes during shutdown. Windows SCM waits for that readiness
  before reporting Running, stores an absolute state path, preserves the
  interface name and installs restart recovery. Service installation
  refuses command-line auth keys; initialize saved state first. Local API
  cancellation is regression-tested; Windows remains compile-verified,
  pending a real-machine SCM/tunnel test.

- Session stream lifecycle: cap each side at 256 active streams, release
  completed transfer channel/open-marker entries and distinguish reset
  from successful end-of-stream so an aborted file cannot publish.
  Regression sends more than 256 files sequentially, checks state cleanup,
  enforces the active-stream cap and resets a complete-length transfer.

- Startup ordering: host/viewer option constructors install file paths,
  recording and callbacks before dispatching streams already queued by an
  eager peer. Desktop incoming files now land under the daemon state
  directory, not its working directory. The local session proof uses
  synchronized frame counters and requires both initial and changed frames.
  Regression tests queue a file and screen frame before host/viewer setup.
  This fixes a concrete setup race; the earlier isolated ACK timeout is
  still not conclusively attributed to it.

- Owned control-map snapshots: copy mutable node and policy state under
  the store lock, then filter/sort outside it; preallocate and compact the
  peer array. Repeated/reordered identical endpoint reports no longer
  rewrite the store or wake readers. Tests cover snapshot isolation and
  no-op behavior. Repeated synthetic benchmarks cut 10,000-node map
  allocations from ~11.1MB to ~2.32MB; mixed-load latency is not uniformly
  improved and changed-endpoint persistence remains the main bottleneck.
  Both before/after results are retained in docs/benchmarks.

- Opt-in sustained HTTP mixed-load harness: map reads, changing endpoint
  updates, real registration attempts with admission limits left enabled,
  cancellable polls and connection churn. Committed 10-second local
  measurements at 1,000 and 10,000 seeded nodes with 16 HTTP workers,
  including per-operation p50/p95/p99 and explicit failure/cancel/429 counts.
  These are synthetic closed-loop measurements, not a production capacity
  claim or an overload/soak test.

- Session backpressure: at most 16MiB of outbound frames can queue before
  sends wait for capacity; close wakes blocked senders. Weighted scheduling
  favors input/screen while allowing chat and bulk completion to progress.
- Safe file reception: incoming transfers default to a 1GiB per-file cap,
  reject unsafe names/invalid sizes and existing targets, write private
  temporary files, check every write/length/digest, then publish atomically
  without replacing a destination. Failed/partial files are removed. The
  sender checks receiver offsets and final digest. Regression tests cover
  acceptance/refusal, traversal, truncation, overflow, existing files,
  private permissions, queue limits and bulk fairness.

- Scale baseline: reproducible map/HTTP/persistence benchmarks at 100,
  1,000 and 10,000 synthetic nodes, plus a race-tested 1,000-watcher
  cancellation regression. Full results and measurement limits are in
  docs/scale-testing.md. This identifies single-lock/whole-file-store
  bottlenecks; it does not claim production fleet capacity.

- Lifecycle hardening: daemon Down cancels and joins its background loops
  before clearing the device and DNS; endpoint reports honor cancellation.
  Map snapshots and device stats use locks. Fresh race testing found the
  shutdown/map race and a separate relay ping/data concurrent-writer race.
  Relay ping and packets now share one writer; disconnect/replacement stops
  that writer, unblocks the reader and removes stale presence records.
  Regression tests exercise status during rejoin/shutdown and relay
  replacement during active forwarding.

- Portal wire hardening: subscribe to Response signals before making
  portal calls, preserve object-path argument types, bound method calls,
  and close sessions after failed setup. A real private D-Bus test enforces
  method signatures, sends responses before method replies, parses real
  structured streams, and transfers a Unix descriptor. This is protocol
  testing, not a GNOME screen-capture pass. Portal host shutdown now kills
  the helper and unblocks capture, closes descriptors and ends the portal
  session; restore tokens are replaced atomically with private permissions.

- Queue payload ownership: asynchronous session sends copy caller buffers
  before returning. A deterministic regression reproduced corruption when
  SendFile reused its chunk buffer; the TCP test now uses random bytes
  instead of a repeating pattern that hid this bug. Oversized payloads
  fail at enqueue time. Mux shutdown wakes its writer under the condition
  lock and interrupts a blocked handshake before taking that lock.
- scripts/local-gates.sh fails on the actual command status and retains
  complete logs. Race tests use explicit -count to bypass cached results.
  Local gates do not certify live desktop platforms or production scale.

- Android live mesh proof: Firebase Test Lab run (virtual Medium
  Phone, API 36) drove the app to State: up with mesh IP 100.64.0.3
  and a populated peer list against the hosted control plane, on
  video and in logcat. Three Android bugs fixed to get there:
  one-time auth keys were presented twice (prepare then Up) and
  rejected on the second join, with the error hidden by a bare down
  broadcast (the daemon now retries keyless as a known node and
  broadcasts the real error); the VpnService lacked the
  BIND_VPN_SERVICE declaration API 30 enforces; wireguard's
  CreateTUNFromFile needs netlink and tun ioctls Android never
  grants apps, replaced by a minimal fd-backed tun device. Plus a
  pre-existing dns Serve/Close data race caught by the new
  regression test under -race.
- Live session proof: mnetd -session-proof joins two in-process
  nodes to any control plane on userspace netstack tunnels (no root,
  no kernel TUN; reuses the Prepare/SetTUN flow built for Android)
  and runs a full session between them through the encrypted mesh:
  real X11 capture from an Xvfb display, dirty-rect delta delivery
  of a mid-run wallpaper swap, nonce-verified chat, a hash-checked
  256 KiB file transfer, and injected text observed reaching the X
  server. Artifacts (frames, summary.json) land in -proof-dir.
- File transfer: incoming files were refused when the receive
  directory did not exist yet; the receiver now creates it.
- Control plane hardening: per-IP rate limits on admin login and
  node join (429 with Retry-After), security headers on every
  response (API responses deny content loading and caching), and an
  append-only audit log at <store>.audit.jsonl covering logins,
  joins, approvals, key and node administration.
- Session authorization: hosts bind the viewer's claimed node name
  to the source mesh IP against the network map, mnetd
  -session-allow restricts hosting to named nodes, and every
  accept, refusal and session end lands in <state>/sessions.jsonl.
- Metrics: GET /admin/api/metrics reports uptime, join and login
  outcomes, rate-limit hits, fleet size and live node count.
- Session resilience: the mux runs an active keepalive (ping every
  15 s, close after 45 s of silence) on both host and viewer, so
  NAT reboots and dead relays no longer leave a session looking
  open; the CLI viewer reconnects on a drop with capped exponential
  backoff and the picture resumes from the host's fresh keyframe.
- Adaptive streaming: keepalive pings now carry a timestamp so both
  sides measure the path round trip; hosts with a tuneable capturer
  (the X11 backend) step JPEG quality between 70, 55 and 40 as the
  RTT crosses 120 ms and 250 ms, reviewed every 5 s.
- Android viewer auto-reconnect: a dropped session redials with
  capped backoff (1 s to 15 s) until the user leaves, and the host's
  fresh keyframe restores the picture.
- Roaming: on Linux the daemon subscribes to rtnetlink link and
  address events and re-resolves its endpoints within seconds of a
  network change (2 s settle debounce), instead of waiting out the
  45 s discovery tick; other platforms keep the interval.
- Host-side session recording: mnetd -session-record DIR writes each
  hosted session as a standalone MJPEG video with 0600 permissions,
  deltas composited so the file is complete; audit lines name it.
- GNOME Wayland capture: new portal backend (internal/portal).
  mnetd negotiates a monitor stream over the XDG ScreenCast portal
  in pure Go (godbus; CreateSession, SelectSources, Start,
  OpenPipeWireRemote behind a testable bus interface) and pumps
  frames out of the granted PipeWire node with a gst-launch helper
  that decodes and scales to raw BGRA. Backend selection: grim
  stays the default on wlroots compositors, the portal takes over
  on compositors without wlr-screencopy (GNOME), MNET_CAPTURE=portal
  forces it; ydotool keeps input on every Wayland path. The
  capturer is tuneable, so adaptive quality applies, and implements
  dirty-rect deltas: GNOME sessions send changed 64x64 tiles instead
  of full frames under the same 60% rule as X11. The grant persists:
  the compositor's restore token is stored 0600 in the state
  directory and later daemon starts restore the selection without
  re-showing the consent dialog while the grant lasts. Negotiation,
  parsing, pump framing, change detection, delta tiling and token
  persistence unit tested; live verification on a real GNOME session
  is still pending (no GNOME in CI or the dev sandbox).
- mnet session log prints the node's hosted-session audit from
  <state>/sessions.jsonl: accept/refuse/end, viewer, mesh IP,
  duration, recording path.
- Windows service mode: mnetd -service install registers the daemon
  with the Service Control Manager (autostart, LocalSystem so it can
  manage the tunnel interface, joins from saved state on every boot),
  -service remove takes it back out, and an unflagged service session
  is detected and routed the same way. Service lifecycle events land
  in the Windows event log. The console path is unchanged: one boot
  path now serves the terminal and the SCM handler. CI cross-compiles
  the Windows and Darwin daemons on every push so the service code
  cannot rot; live verification on a Windows machine is pending.
- CI: GitHub Actions build, vet, race-enabled tests, the secrets
  gate, and the live session proof on every push and PR.

## 0.7.0 - 2026-11-15

Session layer performance and platform coverage.

- Dirty-rect screen deltas: hosts with raw frame access (X11 backend)
  diff frames into 64x64 tiles and send only changed tiles as JPEGs,
  with a full keyframe every 100 frames. Viewers composite tiles onto
  the stored base frame and always surface complete JPEGs, so apps
  need no changes. Deltas arriving before the first keyframe are
  skipped. Diff, encode, composite and viewer resolution unit tested,
  pixel assertions validated against JPEG chroma subsampling.
- Wayland backend: mnetd captures with grim (wlroots screencopy) and
  injects with ydotool (evdev level, compositor agnostic) when
  WAYLAND_DISPLAY is set; X11 path unchanged otherwise. GNOME and
  other compositors without wlr-screencopy still need the
  portal-native D-Bus + PipeWire backend (later).
- Selectable text injection (MNET_TEXT_MODE): keysym typing (X11
  default) now remaps a spare keycode for characters outside the
  current keyboard layout, so arbitrary Unicode types on X11;
  clipboard mode sets the clipboard via xclip/xsel or wl-copy and
  injects Ctrl-V. Falls back to the backend default when the
  clipboard tool is missing.
- README gained a remote desktop quick start.

Known limits unchanged: Android session UI compiled but not
device-tested; Android key text injection out of scope; iOS session
support parked with the Apple account work.

## 0.6.0 - 2026-11-15

The session layer: TeamViewer-style remote desktop over the mesh.
Spec in docs/session-layer.md: typed streams (screen/input/chat/file)
with priority classes multiplexed over one TCP connection between mesh
IPs on port 5909, no second crypto stack (WireGuard already encrypts
end to end), host-side consent.

- internal/session: framing, priority-draining mux, handshake, host and
  viewer logic, sha256-verified file transfer with resume. End to end
  tested over net.Pipe with -race.
- mnetd -session-host -session-auto-accept: Linux desktop hosting via
  X11 capture (JPEG, 1280 long edge) and XTEST input injection.
- mnet session connect / send: terminal viewer with chat, frame saving
  and file push.
- Android: HostSessionService (MediaProjection, consent dialog or
  auto-accept), MnetAccessibilityService (gesture injection while
  hosting), ViewerActivity (frames, touch input, chat), MainActivity
  entries. Key text injection is intentionally out of v1.
- Full Go suite green; release APK (versionCode 8) and AAB rebuilt.

Known limits: X11 backend not yet exercised on a live desktop; Android
session UI compiled but not device-tested; Wayland capture needs a
portal backend; iOS session support is parked with the Apple work.

## 0.5.2 - 2026-11-15

Commit: 0d3639b. Android: stable view ids (R.id.server_url, network_name,
auth_key, device_name, exit_node, connect_button) so Firebase Test Lab
robo scripts can drive a real join; the server field now defaults to the
hosted control plane (https://mnet-control.onrender.com). Play store
graphics in store-assets/. Full Go suite green; release APK (versionCode
7) and AAB rebuilt.

Firebase Test Lab (project mnet-testlab, robo + seeded robo script):
- Pixel 9, API 34, matrix-1zjis8j9p8pi4: PASSED, 3m29s, 9 of 9 script
  actions. The script filled the join form, accepted the VPN consent
  dialog, and the device registered with the LIVE control plane: node
  "robo-test-device" approved in network roboproof, mesh IP 100.64.0.1,
  at 06:19:13 UTC (admin state record kept with the run evidence). The
  one-time auth key was deleted right after the run. Open check: the
  crawl wandered after the script, so no screenshot caught the "up"
  state; next run ends the script with a screenshot + terminate.

## 0.5.1 - 2026-11-15

Firebase Test Lab robo (release APK, matrix-1hOudyo5gvsiv): PASSED 2/2.
Pixel 9 API 34 (1m7s) and Pixel 5 API 30 (1m14s), 10 actions, 3 screens
each: install, launch, UI render, and the connect flow reaches the
Android VPN consent dialog on both.

Commit: see tag v0.5.1. Self-healing rejoin: a node the control plane
forgot (server state wipe, admin delete) re-registers itself with its
saved auth key instead of retrying forever. Relay WebSocket is dropped
when its node is deleted, relay dial failures are logged, and the
control server answers /healthz for hosting platforms. Android release
signing support and an App Bundle build (android/build-aab.sh).

## 0.5.0 - 2026-11-15

Commit: see tag v0.5.0. SSO/OIDC login for joins (authorization code
flow, RS256 ID tokens verified against the issuer JWKS, user identity
recorded on the node), key expiry (180 days, rejoin renews, admin
Expire key), iOS sources no longer need an app group.

## 0.4.0 - 2026-11-15

Commit: 0a444e1, tag v0.4.0. Network ACL policies: rules in the admin
dashboard or GET/PUT /admin/api/acl, enforced per node at map build.

## 0.3.0 - 2026-11-15

Commit: 219a36d, tag v0.3.0. Exit nodes and subnet routing, mobile
exit-node field on Android and iOS, release-build.sh.

## 0.2.0 - 2026-11-15

Tag v0.2.0. Android APK (Gradle-free build, debug signed), iOS sources,
public deployment validation over a pinggy tunnel, STUN endpoint
discovery confirmed live.

## 0.1.0 - 2026-11-15

Tag v0.1.0. First working mesh: control plane, daemon, CLI, relay,
Magic DNS, end-to-end tests green.

## Control API redirect refusal (new source)

The daemon HTTP control client no longer follows redirects. A two-server regression reproduced the old behavior forwarding custom node credential headers and, for 307/308, replaying the enrollment body. New local race tests cover 301/302/303/307/308, same-origin/cross-origin destinations, default/supplied clients, zero requests to redirect destinations, preservation of supplied client configuration and successful direct requests. All redirects now fail, so operators must set the final base URL directly. This is a source finding, not evidence of a live attack. All five jobs passed in [9d6c03d CI](https://github.com/MaisySen/Mnet/actions/runs/43799181639), including native Windows/macOS and three full kernel fixtures with 141/144/167 later loss-profile drops. Downloads and hosted deployment are unchanged.

## Enrolled target binding (new source)

Up and mobile Prepare now reject server/network changes for an existing saved node credential, before configuration writes or network requests. This prevents startup flags from transferring the old credential to a newly selected control server or silently moving the enrolled device. Local race tests cover both entry points, changed server/network/both, unchanged live/disk identity, no requests to either server on rejection and a retry reaching the original server. Fresh unenrolled state may still select its target; server trailing slashes are equivalent, but network selectors compare exactly. Use a separate state directory for another identity. All five jobs passed in [c632d45 CI](https://github.com/MaisySen/Mnet/actions/runs/43799335015), including native Windows/macOS and three full kernel fixtures with 151/143/143 later loss-profile drops. No live changes, downloads or remote enrollment migration.

## Enrollment-response boundary and save recovery (new source)

The control client rejects missing/blank node ID or token in successful join responses before the daemon can overwrite an existing credential. A daemon regression checks unchanged live/disk identity and no published control client after a malformed response. A separate failed-save regression verifies valid remote acceptance remains in memory, no new control client is published before persistence, the old disk state is unchanged and a same-process retry sends the accepted credential and saves/reloads it without rotating the key pair. The save error now explains that enrollment was accepted and storage must be fixed before retry/restart. This does not roll back remote state or guarantee recovery after process exit. All five jobs passed in [c8ff4bb CI](https://github.com/MaisySen/Mnet/actions/runs/43800305333), including native Windows/macOS and three full kernel fixtures with 152/145/136 later loss-profile drops. This is same-process recovery, not a crash-durable transaction.

## Credential-safe diagnostics (new source)

Printable transport failures no longer include request URLs or the underlying transport text that can contain approval/relay credentials. Error cause matching remains available for cancellation and retry classification. The approval wait log points to authenticated local status rather than printing its token-bearing URL. Local tests cover maliciously verbose transport errors, cancellation matching and a real pending join whose local status still offers the approval link without logging its token. All five jobs passed in [c9f1197 CI](https://github.com/MaisySen/Mnet/actions/runs/43801230914), including native Windows/macOS and three full kernel fixtures with 154/142/140 later loss-profile drops. This does not sanitize historical or reverse-proxy logs; no live credential exposure is established.

## Bounded response read (new source)

The control client now reads one byte beyond its existing 4 MiB body bound and explicitly rejects oversized replies instead of silently truncating a valid JSON prefix followed by more bytes. Tests cover an exactly limit-sized valid reply and oversized replies, including trailing whitespace. Response-body read failures use the credential-safe printable error while retaining programmatic cause matching; a failing-reader test also checks body cleanup. All five jobs passed in [8fb969a CI](https://github.com/MaisySen/Mnet/actions/runs/43802190586), including native Windows/macOS and three full kernel fixtures with 131/149/132 later loss-profile drops. The 4 MiB bound already existed; this makes enforcement explicit and does not increase supported fleet size or validate every network-map field.

## Remote error privacy and authorization classification (new source)

Non-success HTTP responses now become status-only errors rather than echoing arbitrary control/proxy JSON messages into daemon logs. Classification occurs before reading a response body, preserving the unauthorized sentinel for malformed or oversized 401 replies. Successful response JSON errors also use a generic printable wrapper while preserving their typed cause. Local tests cover redirect/client/rate-limit/server statuses with credential-echoing and oversized bodies, unauthorized matching and typed decode-error matching. All five jobs passed in [4821002 CI](https://github.com/MaisySen/Mnet/actions/runs/43803217307), including native Windows/macOS and three full kernel fixtures with 137/119/140 later loss-profile drops. This trades remote-body diagnostic detail for safer logs; private server logs remain separate and historical logs are not scrubbed.

## Incomplete saved-enrollment refusal (new source)

Saved credentials now require a non-blank node ID, token, server and network together. Loading or ensuring keys refuses partial/blank enrollment instead of contacting a target with a broken identity. Rejection keeps file bytes and the existing key pair unchanged; valid complete enrollment reloads, and an unenrolled pair remains allowed. Local race tests cover each missing field and blank credentials. All five jobs passed in [929b744 CI](https://github.com/MaisySen/Mnet/actions/runs/43804190569), including native Windows/macOS and three full kernel fixtures with 147/149/143 later loss-profile drops. This is not corruption repair, remote token validation or automatic reenrollment.

## Received authorization-denial withdrawal (new source)

Map/relay 403 is now a separate authorization-denied signal. The daemon withdraws the prior map and changes Up to waiting-approval; a received unapproved map also discards any peers instead of leaving old authorization in place. The existing 401 rejoin path now uses the same withdrawal status handling. Local actual WireGuard IPC regressions cover 403 and unapproved-map responses, peer removal, path-refresh non-restoration and later approved-map/status recovery. HTTP/relay sentinel matching is tested. All five jobs passed in [0aaf471 CI](https://github.com/MaisySen/Mnet/actions/runs/43805223887), including native Windows/macOS denial/recovery IPC tests and three full kernel regression fixtures with 135/128/125 later loss-profile drops. These fixtures did not yet directly inject the new denial case. This does not infer unseen offline revocations or turn transient transport errors into denials.

## Kernel received-denial extension (fixture pending)

The disposable Linux fixture now stops its private control process, marks only its test node b unapproved in the stopped fixture file and restarts the same control endpoint. It requires the running kernel VPN to enter waiting-approval with no peers, fail mesh and LAN reachability, then recover direct access after authenticated reapproval without changing saved identity. This is a fixture-only state injection, not a production unapprove API or a live account change. All five jobs passed in [efaf16e CI](https://github.com/MaisySen/Mnet/actions/runs/43806151062), including three complete kernel fixtures that each directly observed denial/recovery and continued through control-off TCP/loss/cleanup with 141/155/131 later loss-profile drops. Existing IPC denial tests remain verified separately.

## Bounded CLI readiness wait (new source)

`mnet wait [--timeout 60s]` observes authenticated local daemon status until Up with an address, a terminal failure or a bounded deadline. It does not mutate daemon state or change asynchronous Up defaults. Unit tests cover Down/joining/approval/Up transitions, failure/expiry/unknown status, missing address, blocked-request cancellation, approval timeout, HTTP/JSON errors and timeout flag bounds. All five jobs passed in [7b6e9ad CI](https://github.com/MaisySen/Mnet/actions/runs/43808113962), including native Windows/macOS CLI tests and three actual kernel fixture CLI success/failed-startup checks, followed by denial/reapproval, control-off TCP/loss/cleanup with 135/138/140 later loss-profile drops. This is a current-state observation, not per-request completion, peer reachability or physical boot proof.

## CLI approval-timeout kernel extension (pending fixture)

The kernel fixture now runs the actual wait command during denied authorization, requires nonzero timeout without changing waiting-approval state or saved identity, then requires success after authenticated reapproval. All five jobs passed in [e15f818 CI](https://github.com/MaisySen/Mnet/actions/runs/43809073253), including three fresh full kernel fixtures with the actual CLI timeout/recovery assertion and 131/143/123 later loss-profile drops. This keeps timeout separate from cancellation of startup or enrollment.

## Machine-readable status (new source)

`mnet status --json` emits one authenticated local status object without human text, preserving the default human output otherwise. Tests cover escaped names, peers, pending approval, standalone JSON decoding, write failures and argument validation. The kernel fixture now parses the actual CLI output and checks readiness fields plus absence of saved secret fields. All five jobs passed in [fcf0605 CI](https://github.com/MaisySen/Mnet/actions/runs/43810178477), including native Windows/macOS CLI tests and three full kernel JSON-status fixtures with 153/160/149 later loss-profile drops. Status can contain a private token-bearing approval URL, so the output is not a share-safe diagnostic bundle.

## Separate account-session boundary (new internal source)

Internal account sessions bind a verified OIDC provider/issuer/subject to one explicit network membership and expose only that principal's approved/unexpired summaries through the existing store query. Every read rechecks membership; provider replacement, expiry and logout deny use. Sessions are capped at 1,024 with expired reclamation and are separate from operator/device credentials. Local tests cover scope, same-email isolation, another member seeing no owned devices, snapshot copies, revocation, logout replay, provider replacement, expiry equality and capacity. All five jobs passed in [661c77a CI](https://github.com/MaisySen/Mnet/actions/runs/43811262859), including native Windows/macOS session tests and full kernel regression fixtures with 138/142/150 later loss-profile drops. No HTTP route used these tokens at that verified revision; newer HTTP plumbing follows.

## Browser-bound account HTTP flow (new source)

Separate account login/logout/own-device endpoints use OIDC state bound to a private browser cookie, exact provider, nonce and requested network. Matching callbacks are one-use, expiring and capped. Verified callbacks require existing explicit membership before issuing a private eight-hour account cookie. Every device-list read rechecks membership and returns only the verified principal's approved/unexpired summaries. Device approval, email labels, admin cookies, device tokens and query parameters cannot create or widen an account session. Logout revokes the server token. Signed-provider HTTP tests cover no-membership denial, wrong-browser/replay, scope, revocation and logout; internal tests cover expiry/provider/capacity and cookie flags. All five jobs passed in [c86ac3b CI](https://github.com/MaisySen/Mnet/actions/runs/43812377082), including native signed-provider account HTTP checks and three full kernel regression fixtures with 155/128/145 later loss-profile drops. At that revision there was no account page; a newer page follows.

## Member account page (new source)

`/account/` provides a separate network-ID sign-in form, own approved/unexpired device table, refresh and server-side sign-out. The table uses text nodes, not HTML from device names. Sign-out clears private rows/input immediately and generation checks prevent a delayed pre-logout response restoring them. The page explains membership setup, that approval is not live connectivity and that sign-out does not disconnect VPN devices. Local real Chrome exercised the signed-provider browser flow, explicit member device render, populated sign-out, delayed-response suppression and logged-out cookie replay denial. Screenshots were inspected. All five jobs passed in [031be71 CI](https://github.com/MaisySen/Mnet/actions/runs/43813517063), including the isolated real-Chrome account proof and three full kernel regression fixtures with 159/153/173 later loss-profile drops. This is not device enrollment, network creation, account/device management or live deployment.

## Member page edge-state browser checks (pending extension)

The Chrome fixture now verifies HTML-like device names stay literal text, empty-list messaging, a denied session clears private rows, a failed logout does not claim success or restore rows, and a 390px viewport has no horizontal overflow. It then finishes the real server logout/replay test. All five jobs passed in [229c0a3 CI](https://github.com/MaisySen/Mnet/actions/runs/43814532649), including the actual Chrome edge-state step and full kernel regression fixtures with 138/142/127 later loss-profile drops. Narrow/empty screenshots were inspected. Rendering/denied/empty responses are browser-local fixtures, while initial OIDC login and final logout still use the real isolated server. This is not physical mobile proof.

## Provider replacement account revocation (new source)

Replacing or disabling the configured OIDC provider now clears all account sessions and pending account logins. Restoring the old provider object cannot revive them. Account issuance, pending-login creation and reads serialize against provider changes. Keeping the identical provider pointer leaves valid sessions alone. Local tests cover disable/restore, pending/live removal, no revival, scope and existing flow checks. All five jobs passed in [b455910 CI](https://github.com/MaisySen/Mnet/actions/runs/43815622508), including native account checks, real Chrome member proof and full kernel regression fixtures with 135/136/153 later loss-profile drops. This is source behavior only; no live provider configuration was changed.

## Account sign-in recovery page (new source)

Failed/expired account callbacks now return a generic browser recovery page with a same-site return link, retaining the appropriate rejection status. It does not echo provider errors, codes, state, principal or membership details. It warns that an older account session may remain active and that retrying does not grant membership/enroll devices. Local HTTP and real Chrome checks verify the private recovery text and return navigation; the screenshot was inspected. All five jobs passed in [fa00aca CI](https://github.com/MaisySen/Mnet/actions/runs/43816760633), including native checks, real Chrome recovery navigation and three kernel regression fixtures with 118/108/144 later loss-profile drops. No live provider/account was changed.

## Two-device account scope browser extension (pending)

The real signed-provider fixture now includes two owned devices plus same-email/different-subject, different-network, different-issuer and expired records. The actual account JSON and rendered table must contain exactly the two owned approved/unexpired devices, even with attempted scope-widening query parameters. All five jobs passed in [7299060 CI](https://github.com/MaisySen/Mnet/actions/runs/43817883961), including real Chrome scope checks and full kernel regression fixtures with 155/123/147 later loss-profile drops. Desktop/390px screenshots were inspected. This is authenticated account-summary scope proof, not two new physical running VPN devices or fleet capacity.

## Operator membership inspection (new source)

GET `/admin/api/memberships?network_id=...` now returns only explicit grants for an exact existing network to an authenticated operator session. The response is sorted and copied, no-store and contains private issuer/subject identifiers. It does not expose approval provenance or infer membership, and account/device tokens cannot use it. Existing guarded POST grant/revoke behavior remains separate. Local tests cover network scoping, stable order, copies, authentication and revoked grant removal. Full/native CI pending. A later source change adds read-only dashboard inspection, not grant/revoke controls. Actual local Chrome tests cover scoped reads, empty grants, HTML-as-text and stale-response suppression across network change/logout; extended CI is pending.
