# Releases

Every shipped build, newest first. Runbook format: number, time, commit,
what changed. Test counts: full suite green at every tag
(`go test ./...`: bind, control, daemon, dns, stun, mobile all ok).

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
