# Product scope and current gaps

Updated 2026-11-16, Australia/Sydney. This page records the desired experience separately from implementation. See [STATUS](STATUS.md) for exact verification and source versus download/deployment state.

## Desired experience

The user clarified that Mnet should connect multiple devices under one account in one private mesh/network, reconnect in the background and work with device startup. This is a product target, not a claim of finished Tailscale parity.

An architecture blueprint shared as reference on 2026-11-16 describes personal/business use and hosted/self-hosted directions. It is reference material, not an adopted stack, budget, deadline, migration plan or authorization to provision services.

## Multiple devices and one account

Implemented: a network contains separately keyed devices, each with its own identity and mesh IP. Approved, unexpired peers and relay traffic are scoped to that network. Enrollment uses a server URL, network and enrollment key or individual approval. Optional OIDC records the approving display identity on the device. Newer source also preserves the verified issuer/subject privately as approval provenance, without inferring it from old email labels or exposing it in peer maps; that addition passed [c2fe64c CI](https://github.com/MaisySen/Mnet/actions/runs/43780929040).

Not finished: a consumer account/organization ownership and membership model that lets one login safely manage all the user's devices. A stored approving identity is not that model. The current Network type has no account-owner fields. Verified source adds a separate explicit private network/issuer/subject membership gate for future account queries ([7e458a3 CI](https://github.com/MaisySen/Mnet/actions/runs/43783449695)); there is no self-service membership grant or ownership workflow. A separate read-only account login/member page is now in verified source ([7299060 CI](https://github.com/MaisySen/Mnet/actions/runs/43817883961)); it lists only the authenticated principal's approved/unexpired devices after explicit membership. It is not enrollment or device management. Approval provenance alone does not grant membership. A JSON-only, bounded admin-session management endpoint for explicit grant/revoke passed [f42cb58 CI](https://github.com/MaisySen/Mnet/actions/runs/43784350674). That operator endpoint neither authenticates account users nor exposes device lists. New source also adds private exact-network read-only dashboard membership inspection ([e25b70d CI](https://github.com/MaisySen/Mnet/actions/runs/43819448319), all five jobs passed). Newer source adds confirmed removal of an already listed exact principal plus a manual exact-principal grant form ([618d602 CI](https://github.com/MaisySen/Mnet/actions/runs/43841705310)). Grant setup requires trusted provider issuer/subject records, acknowledgement and full target confirmation; it never infers membership from email or approval. [50646a5 CI](https://github.com/MaisySen/Mnet/actions/runs/43843273639) verifies uncertain committed grants and private draft cleanup. Removal affects account queries, not VPN connections; all five jobs passed in [52e2c38 CI](https://github.com/MaisySen/Mnet/actions/runs/43823084397).

Three fresh [799f3a1 kernel fixtures](https://github.com/MaisySen/Mnet/actions/runs/43778473220) exercise three independently keyed VPN devices in one network, all six directed mesh pings and third-device OS TCP, plus a separate LAN host. The later [e0a8f84 CI](https://github.com/MaisySen/Mnet/actions/runs/43780009817) passed three full fixtures that also keep a second network isolated on the same control service. Synthetic control-plane scale fixtures are not evidence of an active-device fleet.

## Startup by platform

| Platform | Implemented | Verified limit / remaining work |
| --- | --- | --- |
| Windows | Automatic-start SCM service uses saved state and requests crash recovery. | Native install/start/API/stop/restart/forced-crash recovery/remove pass. Actual Wintun VPN reconnection and real reboot are not verified. |
| Linux | Daemon accepts saved-state `-up`; a systemd unit is documented. New source adds read-only `-service print-unit` generation (CI pending), not installation/enabling. | Kernel saved Down/Up cycles pass. Three kernel fixtures passed fresh-process saved-state `-up`, stable identity and restored direct VPN/LAN in [c33c6c5 CI](https://github.com/MaisySen/Mnet/actions/runs/43775496938). The later [64eb696 CI](https://github.com/MaisySen/Mnet/actions/runs/43776218766) passed all five jobs and three complete fixtures after test-only deadline adjustments. Automatic installation/enabling and actual systemd boot/reboot are not tested. |
| macOS | Daemon and manual startup; docs suggest a LaunchDaemon. | No packaged/tested launchd setup or actual VPN/reboot proof. |
| Android | Foreground VpnService uses saved preferences and START_STICKY. | No boot receiver is implemented. Automatic reboot startup/OS always-on behavior and physical-device reconnect remain unverified. Prior virtual Test Lab join is not a startup proof. |
| iOS | Network Extension source starts a tunnel manually. | On-demand/automatic startup is not implemented or physically verified. Signed app delivery remains open. |

A desktop service reporting running means its local API is available, not that its VPN has joined. Sleep/wake, loss of connectivity, credential expiry and boot ordering need dedicated end-to-end validation.

## Boundaries

Current builds remain test-only. The bounded Linux LAN fixture requires operator-enabled forwarding and permissive forwarding policy. Exit internet routing is unsupported. Account isolation, real-device validation, durable hosted storage, recovery and independent security review remain work, not implied guarantees.
