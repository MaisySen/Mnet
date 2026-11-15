# Running Mnet in production

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
Administrator with wintun.dll next to it. Service mode is implemented but
still needs live verification on a real Windows machine. Use an explicit
absolute state path consistently, initialize/join it in an elevated console,
then install with `mnetd.exe -state C:\MnetState -service install` and start
with `sc.exe start mnetd`. Never include an auth key in the install command.
Removal: `mnetd.exe -service remove`. Keep the binary and its parent directory
writable only by administrators; LocalSystem executes it at boot. Choose a
state directory whose Windows ACL grants access only to administrators and
SYSTEM. Automatic restart recovery is requested at installation. The SCM
Running status means the local API is bound, not that the mesh has joined.

## Sizing

The relay carries only nodes that cannot find a direct path. Each relayed
node is one WebSocket. Relay capacity has not yet been measured; see
docs/scale-testing.md for the narrower local control-plane measurements.
Direct traffic never touches the control plane.

## Public deployment (validated 2026-11-15)

mnet-control is one static binary with one state file. On any public host:

    mnet-control -listen :443 \
        -base-url https://control.example.com \
        -data /var/lib/mnet/control.json

Put it behind any TLS-terminating reverse proxy (Caddy, nginx) or give it
a public port; the node API, admin dashboard, and WebSocket relay share
the one listener. Set the admin password with MNET_ADMIN_PASSWORD or
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

A node serving as an exit node or subnet router runs:

    mnetd -up -advertise-exit-node
    mnetd -up -advertise-routes 10.9.0.0/24

Linux serving setup is automatic: the daemon writes
/proc/sys/net/ipv4/ip_forward and adds a POSTROUTING MASQUERADE rule for
100.64.0.0/10 with iptables. Keep both on a dedicated node with a systemd
unit; netfilter-persistent is not needed because the daemon reasserts the
rule on every up.

macOS: the daemon sets net.inet.ip.forwarding=1, but masquerading needs a
pf anchor you add once by hand: `nat on en0 from 100.64.0.0/10 to any -> (en0)`
in /etc/pf.conf, then `pfctl -f /etc/pf.conf -e`.

Windows: enable Internet Connection Sharing for the Mnet interface; the
daemon reports this until native forwarding support lands.

Clients pick the exit node with `mnet up --exit-node NAME`. The client
pins host routes for the tunnel endpoint and the control server through
the underlay gateway, then installs 0.0.0.0/1 + 128.0.0.0/1 via mnet0.
The underlay default route stays in place, so `mnet down` restores
ordinary routing without cleanup. Mobile clients skip OS route changes:
the Android VpnService and iOS NetworkExtension builders own the route
table, and the allowed IPs already steer traffic into the tunnel.

## Android

android/build.sh builds android/out/mnet.apk without Gradle: gomobile
for the Go core (all four ABIs), then aapt2 + javac + d8 + zipalign +
apksigner directly. It needs ANDROID_HOME and ANDROID_NDK_HOME set,
JDK 17, and gomobile + gobind in PATH. The debug signing key is
android/debug.keystore (regenerate for real distribution).

## iOS

See ios/README.md: sources and the gomobile build script are complete;
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
