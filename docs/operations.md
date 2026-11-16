# Operating Mnet test builds

Development only. These setup examples are not production certification. Check [STATUS](STATUS.md) for the source/release/deployment split and [native validation](native-platform-validation.md) before relying on a platform.

## Find the right procedure

| Need | Start here |
| --- | --- |
| Check what is actually shipped or verified | [Current status](STATUS.md) |
| Run the control service or a node | [Control plane](#control-plane), [nodes](#nodes) |
| Recover enrollment or inspect startup | [Saved enrollment](#incomplete-saved-enrollment), [save recovery](#enrollment-response-and-local-save-recovery), [readiness wait](#bounded-cli-readiness-wait), [private JSON status](#machine-readable-private-status) |
| Set up read-only member sign-in | [OIDC setup](#sso-login-oidc), [account flow](#account-http-flow), [explicit grants](#operator-membership-management-verified-source) |
| Inspect explicit account grants | [Private operator inspection](#operator-membership-inspection) |
| Understand routes or network limits | [Subnet/exit scope](#exit-nodes-and-subnet-routes), [ingress limits](#reverse-proxies-and-admission-limits) |
| Check test methods and historical results | [Native/kernel validation](native-platform-validation.md), [release history](RELEASES.md) |

## Control plane

Put mnet-control behind HTTPS with any reverse proxy (Caddy is simplest):

```
control.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Then run it as a service:

```ini
# /etc/systemd/system/mnet-control.service
[Unit]
Description=Mnet control plane
After=network.target

[Service]
Environment=MNET_ADMIN_PASSWORD=choose-something-long
ExecStart=/usr/local/bin/mnet-control --listen 127.0.0.1:8080 \
  --base-url https://control.example.com --data /var/lib/mnet/control.json \
  --trusted-proxies 127.0.0.1/32,::1/128
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
```

The dashboard is at https://control.example.com/admin. Back up the private state file and preserve the service configuration and its separately stored secrets. Account/operator sessions are process-local, so they are not restored from the state file. Test restoration before relying on a backup.

## Control API destination

New source refuses every HTTP redirect from the daemon control API, including same-origin redirects and requests made with a supplied HTTP client. Configure the final control base URL directly. An HTTP-to-HTTPS or path redirect is an error, not an automatic credential handoff. This protects node credential headers and enrollment/approval credentials in request bodies or URLs. It does not make plaintext HTTP safe; use HTTPS outside isolated local fixtures. Dashboard browser navigation is separate.

## Enrolled target changes

New source binds a saved node credential to its saved server and network selector. Up and mobile Prepare reject a different non-empty server or network before saving configuration or making a request. A trailing server slash is ignored; network names/IDs are compared exactly, so use the same selector used for enrollment rather than switching aliases. Omitted flags reuse the saved target. Renaming a node at that target is separate. For a different target, use a separate private state directory and enroll a new identity; do not delete the existing identity as a workaround. This is not account switching, automatic migration or remote device removal.

## Enrollment response and local save recovery

New source refuses a join response without both a non-blank node ID and token, before replacing local identity. A valid remote enrollment can still succeed while saving locally fails, including consuming a one-time key. The running daemon keeps the accepted credential in memory for retry; fix local state storage and retry before restarting. It does not publish the new control client before saving. A successful retry is checked by reload in isolated tests. If the process exits before a successful save, the newly accepted credential can be lost and the server may retain the enrollment: inspect the remote device/key state and use an operator recovery path. Do not assume the enrollment was undone or delete the old identity as a workaround. There is no crash-durable distributed transaction.

## Credential-safe diagnostics

New source hides transport-error text when it could contain a complete credential-bearing request URL. Programmatic error matching is retained, but printed status/logs use a generic transport failure. The approval wait log no longer prints its URL, because OIDC approval links contain a device token. Open the approval link from authenticated local status instead. Keep status/identity files private. This does not scrub historical logs, reverse-proxy URL access logs or external service records, and is not an incident report.

## Response size boundary

New source explicitly rejects control responses larger than the existing 4 MiB limit, rather than parsing a silently truncated prefix. Exactly limit-sized valid replies still work. This is not a fleet-capacity claim: a deployment returning larger maps needs a separately designed pagination/size plan. Response-body read failures use credential-safe printed diagnostics and retain their programmatic cause.

## Remote error diagnostics

New source reports the HTTP status for rejected control requests without printing a server/proxy error body that could echo credentials. A 401 remains programmatically unauthorized even when its body is oversized or malformed, preserving rejoin handling. Non-success bodies are closed without being read. Successful JSON decoding failures use a generic printed diagnostic with the typed cause retained for programmatic checks. For server-side detail, inspect private control/proxy logs with care; this client change does not sanitize those logs.

## Incomplete saved enrollment

New source refuses existing enrollment state with only a node ID or token, whitespace-only credentials, or credentials without a saved server/network. It does not generate a replacement identity, erase the original file or treat that state as a fresh install. Keep the original private file and recover a complete backup/operator enrollment record. Fresh key pairs without any node credential remain valid before their first enrollment. This validates presence, not server reachability or the remote validity of the token.

## Received authorization denial

New source withdraws the previous peer map when a map/relay request returns 403 or a successful map explicitly marks this node unapproved. It reports waiting-approval rather than leaving a stale connected state, and path refresh cannot reinstall those withdrawn peers. A later approved map can restore access. A 401 also withdraws authorization before rejoin. Transient transport errors still keep the last received authorization subject to its known expiry. This cannot learn an admin change while every control/relay request is blocked, and does not promise immediate offline revocation.

## Bounded CLI readiness wait

New source adds `mnet wait [--timeout 60s]` for scripts that need a daemon Up observation rather than the asynchronous `mnet up` acknowledgement. Timeout must be positive and at most 30 minutes. Example: `mnet up ...` followed by `mnet wait --timeout 90s` (use the same global `--state DIR` on both if set). Wait only reads authenticated local status. It succeeds with a mesh address when current status is Up; startup failure, expiry, invalid status or unreadable status exits nonzero. Approval/pending/Down without an error waits until the deadline. Timeout does not stop or roll back startup. Inspect `mnet status` for the private approval action.

This observes current state, not completion of a particular Up request: concurrent callers can change the daemon, and an already Up daemon satisfies it. It does not prove peer reachability, a physical boot or background service installation. Default `mnet up` behavior is unchanged.

## Machine-readable private status

New source adds `mnet status --json`, a single JSON object from authenticated local daemon status without human headings. Default human status output is unchanged; unknown status arguments fail. It includes daemon/peer state and may include the private approval URL during enrollment, so do not publish or paste its output into shared logs. It does not include saved private keys, auth keys or node-token fields. The approval URL can itself carry a token; the absence of a token field does not make this output safe to share. Use the same global `--state DIR` as the daemon. This is diagnostics, not proof of peer reachability or readiness of a particular Up request.

## Nodes

Linux with systemd:

New source offers read-only generation: `/absolute/path/to/mnetd -state /var/lib/mnet -service print-unit > mnetd.service`. It prints a saved-enrollment unit and never installs/enables/starts a service or edits identity. Use a stable final daemon binary path, the same dedicated absolute state directory used during enrollment, and review the generated text before installing it through your host's normal service-management process. Enrollment/routing/session/validation overrides and auth keys are refused rather than silently omitted. Paths use a conservative literal character set; whitespace, systemd expansion characters and non-canonical paths reject. No secrets belong in unit files. Keep the executable and parent directories writable only by trusted administrators. Generated defaults request network-online ordering, private umask and restart on failure; neither boot nor VPN readiness is proven by generation. Generator tests, actual CLI no-state-write checks and `systemd-analyze verify` pass in [c3caba0 CI](https://github.com/MaisySen/Mnet/actions/runs/43846988647), all five jobs passed.

The manual example below remains supported:

```ini
# /etc/systemd/system/mnetd.service
[Unit]
Description=Mnet node
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
UMask=0077
ExecStart=/usr/local/bin/mnetd -state /var/lib/mnet -ifname mnet0 -up
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Join once with an auth key:

```bash
mnet --state /var/lib/mnet up --server https://control.example.com \
  --network home --auth-key mnet_...
```

or leave the key off and open the private approval link from authenticated
`mnet status`. Do not paste that credential-bearing URL into shared logs.

macOS: run mnetd with sudo or a LaunchDaemon. Windows: run mnetd.exe as
Administrator with wintun.dll next to it. Basic SCM install/start/stop/restart/crash recovery/removal passes on a disposable native runner; actual Wintun/VPN service operation and physical-host verification remain open. Use an explicit
absolute state path consistently, initialize/join it in an elevated console,
then install with `mnetd.exe -state C:\MnetState -service install` and start
with `sc.exe start mnetd`. Never include an auth key in the install command.
Removal: `mnetd.exe -service remove`. Keep the binary and its parent directory
writable only by administrators; LocalSystem executes it at boot. Choose a
dedicated state directory. Current code establishes a protected DACL for its owner, administrators and SYSTEM; do not use a directory owned by another account. Automatic restart recovery is requested at installation. The SCM
Running status means the local API is bound, not that the mesh has joined.

## Sizing

The relay carries only nodes that cannot find a direct path. Each relayed
node is one WebSocket. Relay capacity has not yet been measured; see
docs/scale-testing.md for the narrower local control-plane measurements.
Direct traffic never touches the control plane.

## Public deployment (historical smoke test, 2026-11-15)

mnet-control is one static binary with one state file. On any public host:

    mnet-control -listen 127.0.0.1:8080 \
        -base-url https://control.example.com \
        -data /var/lib/mnet/control.json

Put it behind a TLS-terminating reverse proxy (Caddy, nginx). The binary does not itself implement TLS; exposing its plain HTTP listener publicly is not a secure HTTPS setup. The node API, admin dashboard, and WebSocket relay share the one listener. Set the admin password with MNET_ADMIN_PASSWORD or
-admin-password, then create networks and auth keys in the dashboard at
https://control.example.com/admin.

Smoke tested end to end through a free pinggy tunnel
(ssh -p 443 -R0:127.0.0.1:18080 a.pinggy.io): two mnetd processes joined
over the public https URL and exchanged WireGuard traffic both ways in
about 7 seconds. scripts/public-validation.sh reproduces this.

Nodes join with:

    mnetd -up -server https://control.example.com -network mynet -auth-key mnet_...

## SSO login (OIDC)

Set MNET_OIDC_ISSUER, MNET_OIDC_CLIENT_ID and MNET_OIDC_CLIENT_SECRET
(or the matching flags) on mnet-control. Also set an explicit admission
policy with `MNET_OIDC_ALLOWED_SUBJECTS` (comma-separated exact subject IDs)
and/or `MNET_OIDC_ALLOWED_EMAIL_DOMAINS` (exact domains, requiring the
provider's signed `email_verified: true`). Startup rejects SSO without a
policy. Matching CLI flags are `-oidc-allowed-subjects` and
`-oidc-allowed-email-domains`. Domain entries do not grant subdomains.
With valid configuration, joins switch from manual approval to SSO.
Register <public base>/oidc/callback as the redirect URL at the provider.
The verifier currently supports RS256 ID tokens only.
The current test evidence is a local signed fake provider,
not a live Google, Microsoft Entra, Okta or Keycloak certification. Auth keys
still bypass SSO, so keep issuing them for servers. The identity from
the login (email, else subject) lands in the node's User field and the
dashboard. Set `-base-url` to the externally reachable HTTP(S) origin.
Approval links and OIDC callbacks always use that configured origin, never
request Host or forwarding headers. Paths, queries, fragments and URL
credentials are rejected in the base URL. Changing a proxy does not change
the callback URL; update the configured origin and provider registration.

## STUN and restrictive networks

Endpoint discovery sends STUN binding requests from the WireGuard socket
itself, so the discovered mapping is the one WireGuard can use. On
networks that filter outbound UDP (some sandboxes and corp networks),
STUN simply finds nothing; the daemon falls back to relay-only operation
and keeps retrying on a 45 s cycle. No configuration is needed either
way.

## Exit nodes and subnet routes

Status: incomplete. Do not rely on exit-node internet routing or subnet routing yet.

The CLI and maps carry `--advertise-exit-node`, `--advertise-routes` and `--exit-node`. WireGuard allowed-IP tests cover advertised CIDRs and selected exit defaults. Those are crypto-routing configuration tests, not complete OS routing evidence.

Current limitations from source review:

- New source explicitly rejects selected/advertised exit routing before enrollment or device creation. The inverted desktop guard and unowned host/default-route replacement helpers were removed, not enabled. Ownership-safe full-tunnel routing remains future work. Rejection regressions and all five jobs passed in [fcbddd1 CI](https://github.com/MaisySen/Mnet/actions/runs/43771745649).
- Newer Linux source installs owned RFC1918 IPv4 subnet routes from permitted peer maps. It refuses overlap with existing non-default routes, uses add rather than replace, tags its own routes with protocol 242 and a per-instance metric, caps owned routes at 256 and input entries at 1024, bounds each command to five seconds, rolls back failed additions, and removes owned routes on map withdrawal/expiry/Down. The bounded kernel fixture passes installation, peer-loopback reachability, saved lifecycle and unrelated-route preservation; no general routing-readiness claim. Non-Linux and public/IPv6 subnet OS routes remain unsupported. An allowed IP alone is not OS routing.
- New source removes the broad unowned MASQUERADE setup. Managed Linux subnet serving accepts at most 256 RFC1918 IPv4 routes, requires operator-enabled IPv4 forwarding, adds uniquely tagged source/destination-scoped MASQUERADE rules that exclude output back into its TUN, and removes only its own exact rules on Down or setup rollback. Shared forwarding settings and unrelated firewall/NAT policy are never changed. Setup failure now fails Up rather than silently leaving an advertised-but-unconfigured node. Local ownership/rollback tests and three fresh separate-LAN kernel fixtures passed in [6735500 CI](https://github.com/MaisySen/Mnet/actions/runs/43770289687). The disabled-forwarding failure path passed three fresh [fcbddd1 fixtures](https://github.com/MaisySen/Mnet/actions/runs/43771745649), including retry after the fixture enables forwarding. Automatic exit serving and non-managed/mobile serving are rejected; macOS/Windows serving is unsupported without modifying their forwarding settings. Existing broad rules from older binaries are not adopted or deleted automatically.
- The old host/default-split replacement helpers have been removed. Safe rollback, control/relay reachability, endpoint changes, selected-peer removal and existing-route preservation need tests before enabling these paths as supported behavior.
- Mobile VPN APIs own their interface routes, but end-to-end exit/subnet operation on physical mobile devices has not been verified.

The established Linux kernel result covers mesh addresses and mesh-route removal. The newer advertised-subnet reachability, lifecycle cleanup and unrelated-route preservation fixture passes; STATUS links its result. Route-only ACL withdrawal/restoration also passed once, including peer and unrelated-route preservation; three fresh full-kernel repetitions passed. Admin expiry with actual route teardown/reapproval and corrected local up status passed three fresh e6ab28d fixtures. Three 6735500 fixtures additionally passed control-off TCP to a separate LAN host without a mesh return route, with owned NAT removal/recreation and preserved unrelated NAT/operator LAN route/forwarding. No exit internet-routing proof is established. Do not apply manual forwarding/firewall changes to a primary machine based on the old routing claims.

## Android

android/build.sh builds android/out/mnet.apk without Gradle: gomobile
for the Go core (all four ABIs), then aapt2 + javac + d8 + zipalign +
apksigner directly. It needs ANDROID_HOME and ANDROID_NDK_HOME set,
JDK 17, and gomobile + gobind in PATH. Regenerate the AAR explicitly for the source revision before both APK and AAB builds; the scripts reuse an existing AAR. Set MNET_RELEASE_KEYSTORE and MNET_RELEASE_PASS_FILE for the existing upload identity. Without release configuration, a per-checkout debug key is used. Never commit signing secrets.

## iOS

See [ios/README.md](../ios/README.md): source integration and the gomobile build script are present;
the xcframework build, signing and on-device testing need macOS, Xcode
and an Apple Developer account.


## Reverse proxies and admission limits

By default the control plane uses the TCP peer address for IP rate limits
and audit events. It ignores `X-Forwarded-For`: a direct caller must not be
able to choose its bucket or falsify the audit source.

Behind a reverse proxy, set `-trusted-proxies` or `MNET_TRUSTED_PROXIES` to
an explicit comma-separated list of the proxy's source CIDRs. Configure
only ranges you control or have verified with the hosting provider, and
ensure the proxy sanitizes or appends the real caller address. Do not use
`0.0.0.0/0` or `::/0`. A misconfigured trust list can reintroduce spoofing.
No proxy ranges are inferred from hosting-provider names.

For trusted direct peers, forwarding chains are read right to left to the
first untrusted address. Earlier caller-supplied entries cannot choose the
source. Invalid inspected hops, oversized headers and chains longer than
32 hops fall back to the TCP peer. IPv4 and IPv6 are supported. Without
proxy configuration, callers behind a proxy share its rate-limit bucket;
configure this deliberately before production use. Malformed CIDRs stop
startup rather than silently widening trust.

Each limiter retains at most 10,000 IP buckets and sweeps hourly-idle
entries at most once per minute. A full table rejects new addresses until
capacity expires; it does not evict active callers to reset their budgets.
These in-process limits are defense in depth, not a distributed DDoS
service. Use network edge limits for public deployments.


### OIDC token validation limits

The verifier requires an exact, case-sensitive configured issuer, an ASCII
subject up to 255 bytes, future expiration, and issued-at no more than one
minute ahead of the server clock (and before expiration). Audience must be
this client alone, either a string or a single-element array; additional
audiences are rejected because no additional-audience trust is configured.
If `azp` is present, it must be this client. RS256 signatures are checked
against the discovered JWKS. Signed negative-claim tests cover these rules.
The reference is OpenID Connect Core 1.0 sections 2 and 3.1.3.7:
https://openid.net/specs/openid-connect-core-1_0.html

SSO admits only identities matching the configured subject or verified
email-domain policy. Both the signed token and the per-login nonce must
validate before approval. The nonce is bound to the one-use login state.
Email policy trusts the configured issuer's email-verification claim; use
exact issuer-local subject IDs when that assurance is insufficient. Keep
SSO disabled and use manual approvals or scoped auth keys if no suitable
policy is known. This implementation has not had an independent security
review.


OIDC discovery must return the exact configured issuer. Provider discovery,
token and JWKS calls have a 15-second total timeout, honor request
cancellation, and never follow redirects. A redirect at the token endpoint
cannot forward the client secret to another destination. Provider error
bodies are not echoed to the browser. Production issuer/endpoints require
HTTPS; plain HTTP is accepted only for loopback local integration tests.


### HTTP ingress bounds

The control listener applies a 5-second header timeout, 30-second request
read timeout, 65-second response write timeout, 90-second keep-alive idle
timeout, and 32KiB header limit (Go's HTTP parser has a small additional
read buffer allowance). The write timeout leaves room for 50-second map
polls. WebSocket relay connections have their own deadlines after upgrade;
TCP tests verify they continue beyond an artificially shortened HTTP
deadline. Slow-header, slow-body and oversized-header tests also run.


### Authenticated rejoin and expiration

Existing node identities require their saved node ID and token on join;
a public WireGuard key alone is never an identity-recovery credential.
Clients send these in `X-Node-ID` and `X-Node-Token`, not the join JSON.
Even a valid network enrollment key cannot take over a known identity
without its saved node credential. If local credentials are lost, an
administrator must remove the stale node before reenrollment, then approve
it again or supply a valid enrollment key.

Expiration also removes approval on rejoin unless a valid enrollment key
authorizes renewal. The saved node token preserves identity, not perpetual
approval. Updated daemons preserve and present their node credentials;
older clients that relied on public-key-only rejoin must be updated.


### State-save failure behavior

Administrative and join changes roll back their changed in-memory entries
if persistence fails. A failed approval cannot leave live authorization
behind an error response; failed enrollment does not consume its one-use
key or retain a new node/address. SSO approval and identity are one save
and one map-version update. Rollback records only changed entries, not a
second full copy of the store.

State writes use a private, unpredictable temporary file in the same
directory, sync and close it before rename, and remove temporary files on
failure. This avoids following the old predictable `.tmp` path. It is not
a database transaction across hosts and does not claim directory-entry
power-loss durability on every filesystem. The entire JSON store is still
rewritten for each real change; the measured scale limitation remains.


### Relay scope and revocation

Relay connections and destinations are scoped to a network, including when
the same public key exists in more than one network. Each forwarded packet
rechecks both nodes' current approval/expiry and the current pair-level ACL.
The destination index is rebuilt after identity changes; routine packet
checks do not scan the whole fleet. Admin expiration detaches the node's
current relay connection. Expired peers are omitted from subsequent maps.

Updated daemons remove missing peers from both WireGuard and the path
selection table, and clear peers before waiting for reauthorization after
an unauthorized control response. Older daemons did not fully apply peer
removals and must be updated for direct-path revocation. Relay-only server
updates cannot remove a key retained by an old client. Already established
direct paths on offline/outdated clients are not instantaneously revoked
by the control plane; online updated clients apply a new map. Updated
daemons also enforce self and peer expiry timestamps already received in
their map using a local deadline guard, independent of subsequent control
responses. Self expiry removes all peers and reports `key-expired`; peer
expiry removes that peer from WireGuard, path selection, status and DNS.
A newly authorized map can restore renewed keys. This relies on the local
clock and process being scheduled, and does not discover an unseen admin
change while offline. Maps with no expiry retain their legacy behavior.
This is not per-packet application-port enforcement. Existing ACLs operate at
peer-pair and advertised-route visibility level.


### Local daemon API authentication

The daemon's local control API requires a random capability stored as
`local-api-token` in its state directory. The CLI reads it from the same
`--state` directory and supplies it in a request header. Reads are protected
as well as up/down controls; browser-origin requests are rejected. The CLI
does not follow API redirects or send this credential to arbitrary hosts.
Malformed or oversized up requests fail before starting work.

Unix sockets still use owner-only permissions. Windows uses loopback TCP
plus the capability, so the state directory must have an administrator/
SYSTEM-only Windows ACL as described above; Unix mode bits do not set a
Windows ACL. Native tests inspect the actual protected DACL; separate-user and physical-host checks remain open. Existing CLI
and daemon binaries must be updated together for capability authentication.
Do not put the token in browser pages, command arguments or logs. Capability authentication and Windows DACL protection are included in v0.8.2-test.1. Later local-expiry/direct-discovery fixes are source-only until a newer release.

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

An internal helper can list approved, unexpired device summaries for an exact network + verified issuer + subject. It excludes same-email/different-subject identities, identical subjects at other issuers, other networks, legacy email-only approvals and expired devices. Returned summaries contain no keys/tokens/endpoints and do not alias stored state. Local race regressions cover persistence, scope, unknown inputs and expiry. All five jobs passed in [11198e8 CI](https://github.com/MaisySen/Mnet/actions/runs/43782589045), including three full kernel fixtures with 135/145/156 later loss-profile drops. This helper originally had no HTTP route; the later separate account flow below authenticates the principal and requires explicit membership before exposing summaries. The helper alone is not ownership, membership or an authorization grant.

## Explicit network membership gate (verified source)

Private control-state membership now binds an exact network/issuer/subject tuple by an explicit internal operator/admin method. Verified approval alone never grants membership. A separate gated device query denies missing/revoked/wrong-network/wrong-issuer/wrong-subject membership and returns only that principal's approved, unexpired devices, not everyone in the network. Grant/revoke persist with rollback on save failure; tests cover restart, tuple collisions, unknown scope and no cross-principal disclosure. The verified membership model was internal-only; a newer admin-only management endpoint is described below. The later account flow below now provides the authenticated account-query route and read-only login page. Member is the only access shape; owner/admin roles remain unfinished. Revoking this membership only closes the account-query gate, not device transport credentials; explicit device expiry is still required to revoke mesh traffic. All five jobs passed in [7e458a3 CI](https://github.com/MaisySen/Mnet/actions/runs/43783449695), including three full kernel fixtures with 139/143/140 later loss-profile drops.

## Operator membership management (verified source)

`POST /admin/api/memberships` accepts `network_id`, `issuer`, `subject` and an explicit boolean `allow`. New source requires each of those exact case-sensitive fields once; duplicates, mixed/alternate-case keys and unknown fields reject before mutation. It requires an unexpired admin session, JSON content type, a 4KiB bounded strict body and no trailing JSON. Browser requests must have the configured same origin when Origin is present and cannot claim cross-site or sibling-site fetch metadata. Non-browser operator clients may omit those browser headers. The principal in the body is the grant target, never the caller's authentication. A later GET/list extension is documented under operator membership inspection, and the separate account login/query below requires explicit membership. There is no self-service membership or owner role. The target principal must be selected from trustworthy identity evidence, not an email guess. Grant/revoke auditing records the network and outcome, not issuer/subject. A failed persistence attempt returns a generic error and rolls back the live membership. Local race tests cover those HTTP boundaries, expired/forged/absent sessions, persisted grants/revokes and snapshot/audit non-disclosure. All five jobs passed in [f42cb58 CI](https://github.com/MaisySen/Mnet/actions/runs/43784350674), including three full kernel fixtures with 167/136/134 later loss-profile drops. This endpoint is not yet in published downloads or the hosted deployment last verified above.

## Consistent admin request guard (verified source)

All existing admin mutations and login now require JSON and the same browser-origin checks used for membership management. Authenticated state/metrics and later membership inspection reads accept GET, ACL accepts GET/PUT, and mutations accept POST only; GET/HEAD/other methods cannot approve, delete, expire or change policy. Headerless non-browser JSON clients remain supported. HTTPS-configured servers now issue the admin cookie with Secure as well as HttpOnly and SameSite=Lax. Local race regressions exercise every registered admin endpoint/method combination, rejected cross-origin requests without state changes, login rejection without sessions, accepted same-origin/script writes and HTTP-versus-HTTPS cookie flags. All five jobs passed in [1614abd CI](https://github.com/MaisySen/Mnet/actions/runs/43785399011), including three complete kernel fixtures with 172/138/158 later loss-profile drops. This does not add user login, device management UI or transport revocation through membership.

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

## Account HTTP flow

New source adds separate account endpoints, not an admin-session or device-token shortcut. The service landing page links to the member view at `/account/` separately from the operator dashboard; it explains that login neither enrolls a device nor grants membership. All five jobs passed in [302e0a6 CI](https://github.com/MaisySen/Mnet/actions/runs/43826840257), including real-Chrome entry navigation:


1. On the configured origin, POST JSON `{"network_id":"exact-network-id"}` (exactly this one case-sensitive field; duplicate/alternate-case selectors are rejected in new source) to `/account/api/login`. OIDC must already be configured. The response supplies `authorization_url`; open it in the same browser, retaining the private login cookie. Unknown networks are not distinguished at start.
2. Register `/account/oidc/callback` as an additional redirect URI with the existing provider before using the flow. The callback validates state, originating browser proof, provider/nonce and explicit network membership. Approval provenance alone is not membership. Operator membership setup remains separate.
3. The callback sets a private account cookie and redirects to `/account/`, the private member page. `/account/api/devices` returns a JSON list of only that principal's approved/unexpired devices. New source also returns `network_id` from the same authorized session and shows it in the member page, not from the login form or request query. It is cleared with private rows on denial/sign-out. Query parameters cannot select another principal/network. Every read rechecks membership. POST JSON to `/account/api/logout` to revoke the current account session.

Login/logout require same-origin browser headers and JSON; methods are fixed. Cookies are HttpOnly, SameSite=Lax, scoped to `/account/`, Secure on HTTPS. Responses use no-store/no-referrer. Pending logins are capped at 1,024 for ten minutes; account sessions at 1,024 for eight hours. Sessions are process-local and disappear on restart. Provider replacement invalidates old sessions. New source also permanently discards all account sessions and pending account logins when a provider is changed/disabled, so restoring the old provider cannot revive them. This does not create membership, approve/enroll devices, revoke mesh traffic when membership is removed, or offer owner/admin roles. New source provides the `/account/` member page: enter the exact network ID, continue through the configured provider, then inspect only your approved/unexpired devices or sign out. It has no device-management or enrollment controls. These are new-source endpoints, not a live deployment/configuration change. Do not put account cookies or login URLs in shared logs.

Account callback failures now offer a generic return-to-sign-in page. Start again in the same browser, then ask the operator to check membership/provider setup if refused again. Failure does not claim to log out an older session; use Sign out on the account page when needed. The rejection status remains non-success and private callback parameters are not echoed.

## Operator membership inspection

New source supports GET `/admin/api/memberships?network_id=EXACT_ID` with a valid operator/admin session. Exactly one case-sensitive query field is accepted; duplicates, unknown/alternate-case fields and malformed query encoding reject rather than selecting a partial/first value. It lists explicit issuer/subject grants for only that network in sorted order; it does not infer grants from approval/email. Unknown/missing networks fail. The response is private no-store principal data, not suitable for member/device routes or shared logs. Account cookies and node tokens are not operator authentication. Guarded POST grant/revoke still requires an explicit allow decision and same-origin JSON; listing does not change any grant or mesh traffic. The dashboard now has a read-only Account membership grants section: choose a network and Inspect grants. Changing networks/signing out clears the private rows; delayed results cannot repaint a different selection or a signed-out view. New source adds a confirmed Remove membership button for a row already loaded from that exact network. It shows issuer/subject and warns that account queries, not VPN connections, are affected. Cancellation leaves the grant intact. The page rereads the actual list after a successful removal; an uncertain response says to inspect again rather than claiming success. New source adds an explicit grant form: select the network, enter the exact issuer/subject from trusted identity-provider records, acknowledge checking that principal and review the full target before confirming. Never infer subject from email/name/device approval. This gives read-only account-summary access, not operator privileges, enrollment or VPN connectivity. Cancellation writes nothing; success reloads grants, and an uncertain response requires inspection before retry. All five jobs passed in [618d602 CI](https://github.com/MaisySen/Mnet/actions/runs/43841705310), including actual dashboard Chrome. All five jobs passed in [52e2c38 CI](https://github.com/MaisySen/Mnet/actions/runs/43823084397), including actual-Chrome removal/cancellation/uncertain-response recovery. Browser tests use actual operator HTTP grants and reads, plus held responses; the HTML-like subject is rendered as text. All five jobs passed in [e25b70d CI](https://github.com/MaisySen/Mnet/actions/runs/43819448319), including real-Chrome inspection, Windows/macOS operator credential-separation tests and three full kernel regressions with 163/127/153 later loss-profile drops. This is disposable fixture evidence, not live account management.

The inspection UI also validates every returned row against the selected network and required non-empty issuer/subject strings. Failed/invalid reads clear old rows and offer a generic retry message rather than transport details. A local browser extension tests failure/retry, malformed responses and reverse completion of two reads of the same network. Those edge responses are browser-local fixtures, not live service behavior; all five jobs passed in [c834830 CI](https://github.com/MaisySen/Mnet/actions/runs/43820460941).

Removal uncertainty includes a committed POST whose response is lost. Inspect grants again to establish current server state before another write. New local browser coverage deliberately commits the real disposable removal then drops only its response, checks the uncertain message, and confirms a new actual GET shows no grant. Stale row callbacks and a selection invalidated during confirmation cannot send a removal. All five jobs passed in [3fd47cf CI](https://github.com/MaisySen/Mnet/actions/runs/43824315289), including this actual-Chrome fixture and three complete kernel regressions with 138/156/161 later loss-profile drops.
