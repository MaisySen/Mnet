# Selected Android framework simulation

## Run

Run `sh scripts/test-android-framework.sh` and `sh scripts/test-android-framework.sh vpn` with JDK17, Python3, Maven and network access to Maven Central/Google Maven. `MVN` can name the Maven executable; `MNET_MAVEN_REPO` selects the dependency cache. The script copies current production sources into a fresh temporary project, resolves pinned dependencies, runs tests and removes the temporary project.

CI caches Maven dependencies using the framework POM as its dependency key, reducing repeated downloads. A cache does not skip compilation/tests, change pinned versions or make a network failure a pass.

The harness uses Robolectric4.18. Its AAR extraction is limited to pinned AndroidX monitor1.8.0 and espresso-idling-resource3.7.0 classes jars. It does not change application builds or release outputs.

## Current coverage

124 cases pass in two fresh local suites (76 selected service/activity +48 VPN service). The new cross-instance revoke/destruction coverage awaits CI. All five jobs passed in [b09858d CI](https://github.com/MaisySen/Mnet/actions/runs/44003438085) before this follow-up.

| Actual application source | Simulated API levels | Cases | Checks |
| --- | --- | --- | --- |
| HostSessionService, SessionConsent, ProjectionGuard, VpnPlatform, CaptureEncoder |24,25,26,33,34|60 (12 per API)|Notification/channel construction, receiver registration, nonsticky restart/stop, missing-consent cleanup, blocking consent cancellation/exact-answer isolation, instance-local auto-accept/reset, synthetic RGBA-to-JPEG row bounds, host input geometry refusal and pointer validation|
| ViewerActivity, ViewerGate, ViewerTap |24,25,26,33|4 (one per API)|Actual asynchronous connection publication, simulated down/up/cancel/drag dispatch, synthetic JPEG callback, no input before a decoded frame, resize cancellation, disconnect clearing and destruction|
| MainActivity |24,25,26,33|12 (three per API)|Managed up/joining/waiting disables disconnect and explains Settings, stale handler calls cannot send stop, mode refresh restores manual stop, down keeps Connect available|
| MnetVpnService, VpnAlwaysOn, VpnPlatform |24,25,26,29,33,34|48 (eight per API)|Manual/managed disconnect and revoke, no-node request, preflight/preparation failure and retry, blocked worker stop/ownership/next-run recovery, system revoke/destruction during preparation and cross-instance gate isolation|

The separate `vpn` mode replaces the fake VPN endpoint with actual application source, while system mode is shadowed and Go/resource values remain fake. It now starts the actual worker for preflight/preparation failure and controlled blocked-preparation cancellation. It does not establish a real VPN interface or execute Go JNI, real Settings transitions or pixels. Removing both always-on service/notification conditions fails six cases; restored source passes24.

Service tests use reflection to set capture/running state rather than start a valid projection. The geometry-refusal latch resets in actual capture setup according to source checks; that restart path is not executed here. Pointer checks preserve omitted-zero coordinates and reject malformed/unsupported/out-of-frame values.

## Explicit replacements

`mobile.Node`, `SessionViewer`, `MnetVpnService.currentNode()` and the accessibility endpoint are test fakes. No Go JNI, real VPN or injected accessibility gesture executes. Viewer frame bytes are synthetic JPEGs, not device screen content. Encoder inputs are synthetic RGBA buffers, not an actual ImageReader. MainActivity uses placeholder resource IDs mapped to Android strings, a fake HostingActivity, direct status-receiver invocation and a fake VPN endpoint. No app string/layout/pixel or system always-on query is validated by its tests. Both removal of disabled-state wiring and independent removal of stale-click rejection fail four cases, while restored code passes.

The manifest is disabled, so receiver registration execution is not proof of signature-permission enforcement.

## Known gaps

- API34 Activity creation reports a missing VirtualDeviceManager listener class in Robolectric4.18. Activity tests explicitly omit API34; service/encoder tests retain it.
- No valid projection, real consent dialog, physical display rotation, actual Android UI pixels, device/emulator, reboot or physical network runs.
- Real accessibility finite checks and removal of down/up-as-click have source/build checks only, not runtime dispatch proof.
- Synthetic JPEG decoding confirms code paths and dimensions, not device color, orientation, heap pressure or playback appearance.
- Simulated API coverage does not declare complete application support for those OS versions.

## Build evidence is separate

At `d0afcb6`, a fresh local current-Go Android AAR built with minimum API24 for armeabi-v7a, arm64-v8a, x86 and x86_64. Full Android34 resource generation/application Java and min-api24 dex compilation against that AAR passed. These build checks do not execute JNI or validate a signed APK, installation or device behavior. A signed local0.8.3-dev/code15 APK was later built from b09858d application source plus its version bump, with the same current shared Go bindings. Package/signature/archive/alignment checks passed and restricted owner-requested delivery completed with matching downloaded SHA256. There is no new public release. Installation and phone runtime checks remain open.


Worker fixtures inject a factory-controlled Go fake. Missing saved config refuses before node creation; preparation errors close/reset ownership and allow another attempt. A latched prepare lets an active duplicate start, stop and refused replacement occur in deterministic order; cancellation prevents tunnel start, then worker exit releases ownership for the next attempt. Latches release in finally and worker completion is bounded while pumping the simulated main looper. A first harness run that blocked that looper timed out and was not counted; corrected fresh36-case run passed. These fixtures do not test cancellation at every native call or promise real-phone recovery.


New revoke/destruction fixtures hold the real worker in fake Go preparation, cancel through the actual lifecycle callback, and confirm the process-wide gate rejects a different service instance until the old worker exits. No canceled tunnel start occurs; afterward that other instance can start and recover. Six simulated API levels add12cases. Removing only onRevoke cleanup causes12failures; restored source passes48VPN +76existing cases with no failures/errors/skips. This still uses fake Go/system mode and does not execute real Android revocation, process death or native cancellation.
