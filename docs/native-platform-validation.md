# Native platform validation

Native CI is a narrower claim than a physical desktop/phone test. Always name the layer that passed.

## What the platform matrix checks

- Windows and macOS build every package on a native runner, not only a cross-compiler.
- Local API lifecycle and capability tests exercise the runner's local transports.
- Private state tests exercise actual file replacement, link rejection and cleanup. Windows tests read the actual security descriptor, require a protected DACL, and reject grants outside the file owner, LocalSystem and local administrators. This is ACL inspection, not an impersonated second-user penetration test.
- Daemon tests exercise state round trips and WireGuard IPC using an in-memory TUN. Local-expiry regressions cover self/peer deadlines, renewal, stale-map refresh, and repeated Up/Down while later control polls are blocked. They do not create an OS VPN interface.
- Windows CI also installs a real SCM service in an offline fixture, checks authenticated local API startup, clean stop/restart, forced-process crash recovery and removal. The script refuses to replace an existing service. This does not create a Wintun interface or test VPN reconnection.
- Session tests exercise a native loopback TCP connection, screen/input/chat framing and a verified 3MiB file. They also test queued startup and repeated stream cleanup. They do not cross an actual VPN interface or a lossy public network.

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
- OS TCP transfer over an actual VPN under packet loss, constrained bandwidth, large files, cancellation and long sessions.
- Fleet/soak tests, persistent production hosting, independent security review and operational recovery.

No release or passing CI run alone closes this list.
