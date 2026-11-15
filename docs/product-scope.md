# Product scope and current gaps

Updated 2026-11-16, Australia/Sydney. This page records the desired experience separately from implementation. See [STATUS](STATUS.md) for exact verification and source versus download/deployment state.

## Desired experience

The user clarified that Mnet should connect multiple devices under one account in one private mesh/network, reconnect in the background and work with device startup. This is a product target, not a claim of finished Tailscale parity.

An architecture blueprint shared as reference on 2026-11-16 describes personal/business use and hosted/self-hosted directions. It is reference material, not an adopted stack, budget, deadline, migration plan or authorization to provision services.

## Multiple devices and one account

Implemented: a network contains separately keyed devices, each with its own identity and mesh IP. Approved, unexpired peers and relay traffic are scoped to that network. Enrollment uses a server URL, network and enrollment key or individual approval. Optional OIDC records the approving identity on the device.

Not finished: a consumer account/organization ownership and membership model that lets one login safely manage all the user's devices. A stored approving identity is not that model. The current Network type has no account-owner/membership fields.

Real kernel fixtures currently exercise two active VPN nodes plus a separate LAN host. Synthetic control-plane scale fixtures are not evidence of an active-device fleet.

## Startup by platform

| Platform | Implemented | Verified limit / remaining work |
| --- | --- | --- |
| Windows | Automatic-start SCM service uses saved state and requests crash recovery. | Native install/start/API/stop/restart/forced-crash recovery/remove pass. Actual Wintun VPN reconnection and real reboot are not verified. |
| Linux | Daemon accepts saved-state `-up`; a systemd unit is documented. | Kernel saved Down/Up cycles pass. Three kernel fixtures passed fresh-process saved-state `-up`, stable identity and restored direct VPN/LAN in [c33c6c5 CI](https://github.com/MaisySen/Mnet/actions/runs/43775496938). The later [64eb696 CI](https://github.com/MaisySen/Mnet/actions/runs/43776218766) passed all five jobs and three complete fixtures after test-only deadline adjustments. Automatic installation/enabling and actual systemd boot/reboot are not tested. |
| macOS | Daemon and manual startup; docs suggest a LaunchDaemon. | No packaged/tested launchd setup or actual VPN/reboot proof. |
| Android | Foreground VpnService uses saved preferences and START_STICKY. | No boot receiver is implemented. Automatic reboot startup/OS always-on behavior and physical-device reconnect remain unverified. Prior virtual Test Lab join is not a startup proof. |
| iOS | Network Extension source starts a tunnel manually. | On-demand/automatic startup is not implemented or physically verified. Signed app delivery remains open. |

A desktop service reporting running means its local API is available, not that its VPN has joined. Sleep/wake, loss of connectivity, credential expiry and boot ordering need dedicated end-to-end validation.

## Boundaries

Current builds remain test-only. The bounded Linux LAN fixture requires operator-enabled forwarding and permissive forwarding policy. Exit internet routing is unsupported. Account isolation, real-device validation, durable hosted storage, recovery and independent security review remain work, not implied guarantees.
