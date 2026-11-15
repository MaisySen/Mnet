# Mnet project status

Updated 2026-11-16, Australia/Sydney. Development/testing only. No production-readiness, parity, security-certification or supported fleet-size claim.

## Source, downloads and deployment are different

| Layer | Recorded state | Evidence / limit |
| --- | --- | --- |
| Published test artifacts | v0.8.2-test.1, source `2bb55f4be60969a855b37fbb6d607f0831542e45`, Go 1.26.11; Android 0.8.2/code 14. | [Release](https://github.com/MaisySen/Mnet/releases/tag/v0.8.2-test.1). Sixteen assets were verified at release. Update CLI and daemon together. |
| Hosted test control plane | Last verified 2026-11-16 at 07:12 AEDT on the same `2bb55f4` source. | This is a dated deployment check, not a live availability promise. Free-host ephemeral disk/restart state loss remains a limitation. |
| Newer source | Known local expiry enforcement, direct-path fixes and bounded Linux subnet-route ownership; development continues beyond the verified commits below. | [Five-job CI](https://github.com/MaisySen/Mnet/actions/runs/43770289687). These newer changes are not in the v0.8.2 downloads or that last verified deployment. |

## Evidence by layer

- Local expiry: real WireGuard IPC verifies removal at known self/peer deadlines, stale-map/path-refresh protection, renewal and repeated lifecycle under blocked control polling. These tests use in-memory TUNs. Offline clients cannot discover an unseen admin revocation.
- Windows: native protected-DACL inspection and private state replacement, local API, expiry, OS-loopback sessions; actual SCM install/start/stop/restart, crash recovery and uninstall on a disposable runner. No Wintun/physical-device claim.
- macOS: native build/private-state/local-API/expiry and loopback-session tests. Actual VPN interface, physical capture/input and consent remain unverified.
- Linux kernel lifecycle: [1d57a88 CI](https://github.com/MaisySen/Mnet/actions/runs/43763017036) also passed two saved-state Down/Up cycles with stable identity/IP and returned direct paths, then 148 actual drops in the transfer profile.
- Linux kernel: two real TUN interfaces in isolated namespaces; mesh addressing/ping; both peers on direct WireGuard UDP; control/relay process stopped before transfers. OS TCP passed 3 x 4MiB normally and 3 x 4MiB with 1% loss plus 20ms delay; receiver hash and sender ACK checked. That run observed 132 underlay packet drops. Down removed interfaces and mesh routes. Controlled CI, not a public-internet soak.
- Proof rig: isolated X11 changed-frame/chat/hash/ACK checks and bounded userspace stress/loss profiles. Its gVisor recovery configuration is proof-only; production sessions use OS TCP.
- Android: signed test artifacts and a prior virtual Firebase Test Lab join proof. Physical VPN/capture/input/reconnect validation remains open.
- iOS: source and shared Go test coverage, not a signed TestFlight or physical-device result.

## Recent source verification

- Direct convergence: an intermittent CI failure was reproduced and addressed with bounded authenticated probes. Three fresh [2d23b6a fixtures](https://github.com/MaisySen/Mnet/actions/runs/43765976816) passed. This closes that bounded reproduction, not arbitrary NAT or production reliability.
- Expiry and reapproval: [e6ab28d CI](https://github.com/MaisySen/Mnet/actions/runs/43767516777) passed all five jobs and three fresh kernel fixtures. Peer/client-route removal blocks reachability until explicit reapproval; a reproduced stale waiting-approval status/link is corrected and asserted.
- Separate LAN forwarding: [6735500 CI](https://github.com/MaisySen/Mnet/actions/runs/43770289687) passed all five jobs and three fresh fixtures with a fourth namespace. Its LAN host has no mesh return route. Control-off VPN-to-LAN OS TCP passed 3 x 4MiB with receiver hash/sender ACK in each fixture. Later mesh loss profiles observed 154/120/114 drops. Two saved Down/Up cycles, route-only ACL changes, expiry/reapproval, owned NAT cleanup and operator LAN route/unrelated NAT/forwarding preservation passed.
- Serving safety: managed Linux RFC1918 IPv4 serving requires operator-enabled forwarding, owns unique destination-scoped NAT rules, and fails Up on setup errors. It never changes shared forwarding settings or adopts/deletes older broad rules. Local ownership/rollback tests pass. A newer actual-kernel failure-path extension for disabled forwarding is pending independent CI.
- Exit routing: newer source explicitly rejects selecting or advertising an exit node before enrollment/device changes. The inverted desktop guard and unsafe unowned host/default-route replacement helpers were removed rather than enabled. Local rejection tests pass; independent CI is pending. Exit internet routing remains unsupported.

## Important open work

- Wider subnet compatibility, restrictive host firewall policies, non-Linux/IPv6/public subnet OS routes, owned exit routing and public-internet/production soak. The bounded LAN fixture explicitly enables forwarding and permits forwarded traffic; it is not an arbitrary host compatibility guarantee.
- Real Windows Wintun and macOS VPN interfaces, second-user Windows access tests, physical capture/input/permission flows, GNOME portal and physical mobile behavior.
- Larger/cancelled transfers, bandwidth limits, long sessions, sleep/wake, sustained churn and fleet/relay capacity.
- Persistent production storage, recovery practice and independent security review.
- Store-policy/data-disclosure review before any publication. Store copy is a draft.

See [native validation](native-platform-validation.md), [reliability](session-reliability.md), [operations](operations.md) and [scale measurements](scale-testing.md) for methods and exact limits.
