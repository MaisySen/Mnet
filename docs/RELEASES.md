# Releases

Every shipped build, newest first. Runbook format: number, time, commit,
what changed. Test counts: full suite green at every tag
(`go test ./...`: bind, control, daemon, dns, stun, mobile all ok).

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
