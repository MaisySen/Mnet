# Releases

Every shipped build, newest first. Runbook format: number, time, commit,
what changed. Test counts: full suite green at every tag
(`go test ./...`: bind, control, daemon, dns, stun, mobile all ok).

## Unreleased (0.7.1 draft)

Live proofs and the fixes they exposed.

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
  of full frames under the same 60% rule as X11. Negotiation,
  parsing, pump framing, change detection and delta tiling unit
  tested; live verification on a real GNOME session is still pending
  (no GNOME in CI or the dev sandbox).
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
