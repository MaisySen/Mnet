# Selected Android framework simulation

Run `sh scripts/test-android-framework.sh` with JDK17, Python3, Maven and network access to Maven Central/Google Maven. `MVN` can name the Maven executable; `MNET_MAVEN_REPO` can select its local dependency cache. The script copies production sources to a fresh temporary project, resolves pinned dependencies and runs tests. It removes the temporary project afterward.

## What executes

The real `HostSessionService`, `SessionConsent`, `ProjectionGuard` and `VpnPlatform` Java sources run on Robolectric4.18 simulated API24,25,26,33,34. Six tests per API cover notification construction/channel lookup and receiver registration, non-sticky restart/stop, invalid-consent cleanup and stop unblocking an actual service consent waiter. Thirty cases pass locally. The original20 cases passed all-five e2d03a6 CI; the added10 cases are pending. Added cases exercise service broadcast/exact-answer isolation and instance-local auto-accept/stop reset, using reflected service state rather than starting a valid projection.

## What is replaced or not covered

- `mobile.Node`, `MnetVpnService.currentNode()` and the accessibility entry are explicit test fakes. No Go JNI, active VPN or injected input runs.
- The manifest is disabled. Receiver registration execution is not proof of actual Android signature-permission enforcement.
- No valid projection, captured frame, real consent dialog, Activity UI, screen decoding, device/emulator, rotation, reboot or physical network runs.
- SDK simulation is not a declaration that the complete application supports those OS versions.

The Maven AAR runtime extraction in the script is limited to the pinned AndroidX monitor1.8.0 and espresso-idling-resource3.7.0 classes jars required by Robolectric. Main app builds and release outputs are untouched.
