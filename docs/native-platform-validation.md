# Native platform validation

Native CI is a narrower claim than a physical desktop/phone test. Always name the layer that passed.

## What the platform matrix checks

- Windows and macOS build every package on a native runner, not only a cross-compiler.
- Local API lifecycle and capability tests exercise the runner's local transports.
- Private state tests exercise actual file replacement, link rejection and cleanup. Windows tests read the actual security descriptor, require a protected DACL, and reject grants outside the file owner, LocalSystem and local administrators. This is ACL inspection, not an impersonated second-user penetration test.
- Daemon tests exercise state round trips and WireGuard IPC using an in-memory TUN. Local-expiry regressions cover self/peer deadlines, renewal, stale-map refresh, and repeated Up/Down while later control polls are blocked. They do not create an OS VPN interface.
- Windows CI also installs a real SCM service in an offline fixture, checks authenticated local API startup, clean stop/restart, forced-process crash recovery and removal. The script refuses to replace an existing service. This does not create a Wintun interface or test VPN reconnection.
- Linux kernel CI creates two real TUN interfaces in isolated network namespaces, checks assigned addresses and mesh ping, transfers 3 x 4MiB OS TCP payloads normally and 3 x 4MiB under 1% loss plus 20ms delay with sender ACK and receiver hash, asserts direct UDP endpoints, stops the control/relay process before transfers, requires actual nonzero drop counters, then checks interface and mesh-route removal. This is a controlled runner fixture, not physical hardware, the public internet, arbitrary-loss or long-session proof.
- Session tests exercise a native loopback TCP connection, screen/input/chat framing and a verified 3MiB file. They also test queued startup and repeated stream cleanup. They do not cross an actual VPN interface or a lossy public network.

Evidence for the combined direct-path check: [c4a1bcd, all five jobs](https://github.com/MaisySen/Mnet/actions/runs/43760903768). Newer source is distinct from published artifacts; see [STATUS](STATUS.md).

Bounded Linux RFC1918 subnet routing: [8fcaf91 CI](https://github.com/MaisySen/Mnet/actions/runs/43764310572) checks an advertised prefix reaches an address on the peer loopback, owned-route cleanup/reinstallation across two saved-state cycles, and preservation of an unrelated underlay route. This is not forwarding through a separate LAN host, exit routing, IPv6 or non-Linux subnet routing.

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

A later local regression reproduced stale last_error after a successful retry. New source clears errors when a fresh Up attempt starts, preserving errors raised during that attempt. The stronger kernel retry assertion and native regression passed all five jobs and three fresh [4127209 fixtures](https://github.com/MaisySen/Mnet/actions/runs/43772484156), including 140/186/158 later loss-profile drops.

New source adds `mnet up --reset-routing` (while down) to clear saved serving/exit roles without deleting identity. Ordinary saved-state Up keeps its previous routing configuration. Reset cannot be combined with new routing roles. A local API regression reproduced the missing recovery path before this change; local and kernel checks for the new path are being added, with independent CI pending.
