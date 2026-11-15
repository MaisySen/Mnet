# Operating Mnet test builds

Development only. These setup examples are not production certification. Check [STATUS](STATUS.md) for the source/release/deployment split and [native validation](native-platform-validation.md) before relying on a platform.

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

The dashboard is at https://control.example.com/admin. The state file is
the only thing to back up.

## Nodes

Linux with systemd:

```ini
# /etc/systemd/system/mnetd.service
[Unit]
Description=Mnet node
After=network-online.target

[Service]
ExecStart=/usr/local/bin/mnetd --state /var/lib/mnet --up \
  --server https://control.example.com --network home
Restart=always

[Install]
WantedBy=multi-user.target
```

Join once with an auth key:

```bash
mnet --state /var/lib/mnet up --server https://control.example.com \
  --network home --auth-key mnet_...
```

or leave the key off and approve the device from the dashboard link the
CLI prints.

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

- `applyExitNode` skips `ManagedIface=true`, the desktop setting, although its intended behavior is to skip mobile-managed interfaces. This guard needs correction with real route regressions.
- Selected subnet CIDRs are not installed as owned OS routes by the current interface setup. An allowed IP alone does not route an OS packet into the tunnel.
- Linux serving enables IPv4 forwarding and a broad mesh-source MASQUERADE rule; ownership-safe restoration/removal on Down is not implemented. macOS sets forwarding before reporting that a manual pf anchor is needed. Windows reports manual ICS setup.
- Existing helpers can replace host/default-split routes without recording prior ownership. Safe rollback, control/relay reachability, endpoint changes, selected-peer removal and existing-route preservation need tests before enabling these paths as supported behavior.
- Mobile VPN APIs own their interface routes, but end-to-end exit/subnet operation on physical mobile devices has not been verified.

The passing Linux kernel fixture covers mesh addresses and mesh-route removal only. It does not close any exit/subnet limitations above. Do not apply manual forwarding/firewall changes to a primary machine based on the old routing claims.

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
