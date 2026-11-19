# Product scope and current gaps

Updated 2026-11-19, Australia/Sydney. This page separates the intended consumer product from current engineering builds. [STATUS](STATUS.md) records individual commits/checks; the whole-product plan remains the design direction, not proof that these gates passed.

## Product direction: a Multies-only companion

Mnet is intended to be a downloadable companion for eligible Multies users, not an independent public remote-access service. Multies supplies identity, workspace membership and shared product/navigation context. Mnet supplies native device connectivity, capture/input and OS permission handling. There is no intended second signup, guest enrollment or parallel account ownership model.

The consumer journey is:

1. Install the companion and choose **Continue with Multies**.
2. Complete Multies sign-in and, when necessary, choose an eligible workspace. Do not type a server URL, network name or enrollment key.
3. Review the device identity and relevant OS permission disclosure. Declining an optional capability keeps unrelated supported capabilities available.
4. See eligible devices within that workspace, with honest connecting, offline, approval-required and unavailable states.
5. Request a session, get host view approval, and request control separately. View approval must not silently grant control, file transfer or recording.
6. Keep a visible stop action. End, revocation or permission loss must stop the affected real resources, not merely change a UI label.

These are requirements, not descriptions of the current APK. Store download visibility is a separate distribution decision; Multies-only eligibility does not by itself change who can see a store listing or authorize a wider beta.

## Current capability and release gates

| Journey | Current evidence | What still blocks the consumer claim |
| --- | --- | --- |
| Setup and devices | A clickable 28-state prototype exists; current builds retain manual engineering enrollment. | Real Multies sign-in, eligible-workspace/device data, no manual-network fallback, and end-to-end failure/recovery checks. Prototype state is simulated. |
| Identity and workspace | Mnet-side exact workspace/device/capability policy and native intent binding have local tests but no production verifier or caller. | A scoped Multies authorization bridge and revocation contract. Multies source is not changed. Identity-provider login, MCP OAuth or an old enrollment key cannot substitute. |
| VPN and DNS | Kernel fixtures and individual Android API34/API36 checks have passed at recorded commits. | A reliable complete Android matrix, default-network separate-app DNS, physical recovery/reconnect and supported platform lifecycle evidence. Current Android native matrix remains failed. |
| Attended view/control | Current transport has host admission and OS capture/accessibility handling. New Android source has a separate host-side control gate, with view-only default/auto-accept and explicit grant/revoke. The Android viewer request/status negotiation is also implemented with protocol/loopback/framework evidence, not current native or physical acceptance. The Multies consent model remains inactive. | Physical/OEM verification of the Android gate, adversarial native stale-answer checks, authenticated Multies workspace/session binding, and cross-platform enforcement/cleanup. Existing desktop combined admission is not the target separate-consent journey. |
| Incoming files | Configured desktop destinations use bounded transfer, portable names, digest validation and atomic no-overwrite publication. | Real destination selection and approval. Empty destinations refuse; current mobile-host and viewer default file reception is unavailable. A directory setting is not per-file consent or Multies authority. |
| Stop and recovery | Local transport/worker/capture cleanup regressions exist. | Physical phone recovery, OS permission revocation, sleep/reboot and membership-loss behavior across the real integrated journey. Local Up is not traffic proof. |
| Distribution | Existing test artifacts remain separate from current source. | No new production or wider beta release is implied by source/CI. Signed iOS delivery and broader rollout readiness remain open. |

The [current source summary](STATUS.md) and [Multies brand/UI checkpoint](multies-brand-alignment.md) separate implemented screen/recovery work from native/device acceptance. [Android request/status negotiation](android-control-negotiation.md) is implemented; its prior placement as wholly missing must not hide that source milestone or imply its native gates passed.

The [Multies boundary](multies-boundary.md) explains the inactive foundations and required bridge. Legacy test enrollment and account administration below describe engineering capability only; they do not meet these release gates. No mock verifier or successful prototype screen may be presented as live authorization.

## Existing engineering model and evidence

### Multiple devices and one account

Implemented: a network contains separately keyed devices, each with its own identity and mesh IP. Approved, unexpired peers and relay traffic are scoped to that network. Enrollment uses a server URL, network and enrollment key or individual approval. Optional OIDC records the approving display identity on the device. Newer source also preserves the verified issuer/subject privately as approval provenance, without inferring it from old email labels or exposing it in peer maps; that addition passed [c2fe64c CI](https://github.com/MaisySen/Mnet/actions/runs/43780929040).

Not finished: a consumer account/organization ownership and membership model that lets one login safely manage all the user's devices. A stored approving identity is not that model. The current Network type has no account-owner fields. Verified source adds a separate explicit private network/issuer/subject membership gate for future account queries ([7e458a3 CI](https://github.com/MaisySen/Mnet/actions/runs/43783449695)); there is no self-service membership grant or ownership workflow. A separate read-only account login/member page is now in verified source ([7299060 CI](https://github.com/MaisySen/Mnet/actions/runs/43817883961)); it lists only the authenticated principal's approved/unexpired devices after explicit membership. It is not enrollment or device management. Approval provenance alone does not grant membership. A JSON-only, bounded admin-session management endpoint for explicit grant/revoke passed [f42cb58 CI](https://github.com/MaisySen/Mnet/actions/runs/43784350674). That operator endpoint neither authenticates account users nor exposes device lists. New source also adds private exact-network read-only dashboard membership inspection ([e25b70d CI](https://github.com/MaisySen/Mnet/actions/runs/43819448319), all five jobs passed). Newer source adds confirmed removal of an already listed exact principal plus a manual exact-principal grant form ([618d602 CI](https://github.com/MaisySen/Mnet/actions/runs/43841705310)). Grant setup requires trusted provider issuer/subject records, acknowledgement and full target confirmation; it never infers membership from email or approval. [50646a5 CI](https://github.com/MaisySen/Mnet/actions/runs/43843273639) verifies uncertain committed grants and private draft cleanup. Removal affects account queries, not VPN connections; all five jobs passed in [52e2c38 CI](https://github.com/MaisySen/Mnet/actions/runs/43823084397).

Three fresh [799f3a1 kernel fixtures](https://github.com/MaisySen/Mnet/actions/runs/43778473220) exercise three independently keyed VPN devices in one network, all six directed mesh pings and third-device OS TCP, plus a separate LAN host. The later [e0a8f84 CI](https://github.com/MaisySen/Mnet/actions/runs/43780009817) passed three full fixtures that also keep a second network isolated on the same control service. Synthetic control-plane scale fixtures are not evidence of an active-device fleet.

### Startup by platform

| Platform | Implemented | Verified limit / remaining work |
| --- | --- | --- |
| Windows | Automatic-start SCM service uses saved state and requests crash recovery. | Native install/start/API/stop/restart/forced-crash recovery/remove pass. Actual Wintun VPN reconnection and real reboot are not verified. |
| Linux | Daemon accepts saved-state `-up`; a systemd unit is documented. New source adds read-only `-service print-unit` generation ([c3caba0 CI](https://github.com/MaisySen/Mnet/actions/runs/43846988647), all five jobs passed), not installation/enabling. | Kernel saved Down/Up cycles pass. Three kernel fixtures passed fresh-process saved-state `-up`, stable identity and restored direct VPN/LAN in [c33c6c5 CI](https://github.com/MaisySen/Mnet/actions/runs/43775496938). The later [64eb696 CI](https://github.com/MaisySen/Mnet/actions/runs/43776218766) passed all five jobs and three complete fixtures after test-only deadline adjustments. Actual runtime systemd start/stop/crash recovery with saved identity, direct VPN and LAN passed in [6fc9839 CI](https://github.com/MaisySen/Mnet/actions/runs/43847999930). Automatic installation/enabling and OS boot/reboot are not tested. |
| macOS | Automatic utun selection uses actual kernel name; saved-state console and read-only generated launchd configuration. | [60e7adc CI](https://github.com/MaisySen/Mnet/actions/runs/43872470943) passed both explicit/automatic native kernel fixtures: actual address/MTU/route, human/JSON CLI, bidirectional TCP to a same-host proof-only userspace WireGuard peer, console/launchd crash/reload recovery, received denial/reapproval and known-expiry enforcement with control/relay offline. No automatic host install, two kernel hosts, public internet/NAT or physical reboot proof. |
| Android | Foreground VpnService uses saved preferences and START_STICKY. | OS-managed always-on/reboot and physical reconnect remain unverified. No boot receiver is implemented; Android guidance prefers system-managed always-on, not an app boot receiver. Startup config, worker ownership and permanent disposal passed their recorded source/native checks, not automatic-startup proof. Prior virtual Test Lab join is not a startup proof. |
| iOS | Network Extension source starts a tunnel manually. | On-demand/automatic startup is not implemented or physically verified. Signed app delivery remains open. |

A desktop service reporting running means its local API is available, not that its VPN has joined. Sleep/wake, loss of connectivity, credential expiry and boot ordering need dedicated end-to-end validation.

### Engineering boundaries

Current builds remain test-only. The bounded Linux LAN fixture requires operator-enabled forwarding and permissive forwarding policy. Exit internet routing is unsupported. Account isolation, real-device validation, durable hosted storage, recovery and independent security review remain work, not implied guarantees.

### Historical Android startup follow-up

[Android VPN guidance](https://developer.android.com/develop/connectivity/vpn) says always-on can start the VPN at boot, must reuse saved configuration and requires adapting disconnect controls. [VpnService reference](https://developer.android.com/reference/android/net/VpnService) says the always-on metadata defaults true when absent, with isAlwaysOn available from API29. [Enterprise guidance](https://developer.android.com/work/dpc/network-telephony) explicitly recommends the system lifecycle rather than an app boot receiver. The current manifest does not opt out; that is not verification that all always-on UX/reconnect paths work. Do not claim boot support from START_STICKY. Worker ownership and permanent disposal now have source/JVM/native evidence as recorded in [native validation](native-platform-validation.md). Follow-up includes disconnect-control behavior and physical cancellation/reconnect/always-on/reboot testing. New API-level guards have local fake/build checks, not device verification.


### Earlier Android always-on and lifecycle validation

API29+ source uses VpnService.isAlwaysOn to omit notification disconnect, disable connected main-screen disconnect, explain Settings ownership and ignore stale service disconnect intents. Manual mode retains disconnect. System revoke/destroy/failure cleanup is unchanged; the app does not change always-on/lockdown settings. API24-28 detection remains unknown with earlier behavior, rather than inferring mode from a null restart intent.

The current framework suites execute actual MainActivity and selected VPN service/worker paths with fake Go/system mode, placeholder resources and direct status invocation. They pass136cases:88 selected service/activity and48 VPN cases. Worker checks cover missing-config preflight, preparation failure/retry, canceled-worker ownership and cross-instance replacement after revoke/destruction. This is not actual system mode, Settings transition, app-string/layout/pixel, JNI/interface, physical reconnect/reboot or receiver-security proof. [032dbc2 CI](https://github.com/MaisySen/Mnet/actions/runs/44005635066) passed all five jobs before the new accessibility disclosure coverage; new follow-up CI is pending. Published downloads remain unchanged.
