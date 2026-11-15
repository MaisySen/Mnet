# Mnet project status

Updated 2026-11-16, Australia/Sydney. Development/testing only. No production-readiness, parity, security-certification or supported fleet-size claim.

## Source, downloads and deployment are different

| Layer | Recorded state | Evidence / limit |
| --- | --- | --- |
| Published test artifacts | v0.8.2-test.1, source `2bb55f4be60969a855b37fbb6d607f0831542e45`, Go 1.26.11; Android 0.8.2/code 14. | [Release](https://github.com/MaisySen/Mnet/releases/tag/v0.8.2-test.1). Sixteen assets were verified at release. Update CLI and daemon together. |
| Hosted test control plane | Last verified 2026-11-16 at 07:12 AEDT on the same `2bb55f4` source. | This is a dated deployment check, not a live availability promise. Free-host ephemeral disk/restart state loss remains a limitation. |
| Newer source | Known local expiry enforcement, direct-path fixes and bounded Linux subnet-route ownership; development continues beyond the verified commits below. | [Five-job CI](https://github.com/MaisySen/Mnet/actions/runs/43760903768). These newer changes are not in the v0.8.2 downloads or that last verified deployment. |

## Evidence by layer

- Local expiry: real WireGuard IPC verifies removal at known self/peer deadlines, stale-map/path-refresh protection, renewal and repeated lifecycle under blocked control polling. These tests use in-memory TUNs. Offline clients cannot discover an unseen admin revocation.
- Windows: native protected-DACL inspection and private state replacement, local API, expiry, OS-loopback sessions; actual SCM install/start/stop/restart, crash recovery and uninstall on a disposable runner. No Wintun/physical-device claim.
- macOS: native build/private-state/local-API/expiry and loopback-session tests. Actual VPN interface, physical capture/input and consent remain unverified.
- Linux kernel lifecycle: [1d57a88 CI](https://github.com/MaisySen/Mnet/actions/runs/43763017036) also passed two saved-state Down/Up cycles with stable identity/IP and returned direct paths, then 148 actual drops in the transfer profile.
- Linux kernel: two real TUN interfaces in isolated namespaces; mesh addressing/ping; both peers on direct WireGuard UDP; control/relay process stopped before transfers. OS TCP passed 3 x 4MiB normally and 3 x 4MiB with 1% loss plus 20ms delay; receiver hash and sender ACK checked. That run observed 132 underlay packet drops. Down removed interfaces and mesh routes. Controlled CI, not a public-internet soak.
- Proof rig: isolated X11 changed-frame/chat/hash/ACK checks and bounded userspace stress/loss profiles. Its gVisor recovery configuration is proof-only; production sessions use OS TCP.
- Android: signed test artifacts and a prior virtual Firebase Test Lab join proof. Physical VPN/capture/input/reconnect validation remains open.
- iOS: source and shared Go test coverage, not a signed TestFlight or physical-device result.

## Important open work

- A later repeated kernel run exposed intermittent direct convergence despite valid candidates. Earlier successful runs remain evidence, not a repeatability guarantee. A bounded authenticated handshake-probe window passed once with route-only ACL withdrawal/restoration in [7a72378 CI](https://github.com/MaisySen/Mnet/actions/runs/43765437995). Three fresh full-kernel runs passed in [2d23b6a CI](https://github.com/MaisySen/Mnet/actions/runs/43765976816), including direct convergence, saved lifecycle, route-only ACL changes, control-off OS TCP/loss and cleanup. This closes the reproduced bounded regression, not arbitrary NAT or production reliability.

- Exit/subnet OS route correctness and ownership-safe cleanup. A bounded Linux RFC1918 subnet-route owner passed [8fcaf91 kernel CI](https://github.com/MaisySen/Mnet/actions/runs/43764310572): owned route installation, peer-loopback subnet reachability, saved lifecycle and unrelated-route preservation. Actual route-only ACL withdrawal/restoration also passed in 7a72378; three fresh full-kernel fixtures passed in 2d23b6a. Actual admin-expiry/route teardown and reapproval passed three fresh fixtures in [0d56bc7 CI](https://github.com/MaisySen/Mnet/actions/runs/43766698566), with 154/152/161 actual drops in the later loss profiles. That fixture initially did not assert the local status after reapproval. A failing local regression then found a stale waiting-approval status/link despite restored traffic; its correction and stronger kernel assertion passed all five jobs and three fresh fixtures in [e6ab28d CI](https://github.com/MaisySen/Mnet/actions/runs/43767516777), with 136/154/164 actual drops. That result is not external LAN forwarding or exit internet routing. New source removes the broad unowned serving rule: Linux private-IPv4 serving now requires operator-enabled forwarding, adds only unique destination-scoped NAT rules, cleans them on Down and fails Up on setup errors. Local ownership/rollback tests pass; a separate-LAN kernel fixture is pending. Automatic exit/mobile serving is explicitly rejected rather than mutating shared forwarding settings. Current desktop exit setup is skipped by an inverted `ManagedIface` guard; mesh-only tests do not validate it.
- Real Windows Wintun and macOS VPN interfaces, second-user Windows access tests, physical capture/input/permission flows, GNOME portal and physical mobile behavior.
- Larger/cancelled transfers, bandwidth limits, long sessions, sleep/wake, sustained churn and fleet/relay capacity.
- Persistent production storage, recovery practice and independent security review.
- Store-policy/data-disclosure review before any publication. Store copy is a draft.

See [native validation](native-platform-validation.md), [reliability](session-reliability.md), [operations](operations.md) and [scale measurements](scale-testing.md) for methods and exact limits.
