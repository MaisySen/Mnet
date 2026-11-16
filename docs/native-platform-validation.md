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

Verified OIDC approval now stores issuer and subject separately from the display email in private control state. No principal is inferred for historical email-only approvals. Identity provenance persists atomically with approval, rolls back on save failure and is removed with the device. It is not added to peer-facing node maps. Local persistence, legacy-unknown, issuer-distinction, non-disclosure and failed-write regressions pass. This is account groundwork, not account ownership, membership, authorization based on email, a login dashboard or automatic device grouping. All five jobs passed in [c2fe64c CI](https://github.com/MaisySen/Mnet/actions/runs/43780929040), including three complete kernel fixtures with 131/144/138 later loss-profile drops. A further signed-provider callback regression now asserts the private issuer/subject was recorded and excluded from the returned map; independent CI for that extra assertion is pending.
