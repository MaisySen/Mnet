# Selected Android framework simulation

Run `sh scripts/test-android-framework.sh` with JDK17, Python3, Maven and network access to Maven Central/Google Maven. `MVN` can name the Maven executable; `MNET_MAVEN_REPO` can select its local dependency cache. The script copies production sources to a fresh temporary project, resolves pinned dependencies and runs tests. It removes the temporary project afterward.

## What executes

The real `HostSessionService`, `SessionConsent`, `ProjectionGuard` and `VpnPlatform` Java sources run on Robolectric4.18 simulated API24,25,26,33,34. Nine tests per API cover notification construction/channel lookup and receiver registration, non-sticky restart/stop, invalid-consent cleanup and stop unblocking an actual service consent waiter. Forty-five cases pass locally. All49 cases, including the Activity extension below, passed [904a1af CI](https://github.com/MaisySen/Mnet/actions/runs/43919715270), all five jobs. Final-row cases include direct buffers without trailing padding and nonzero offsets. Encoder cases check padded/exact row decoding dimensions and invalid layouts/buffers, not device colors or heap pressure. Added cases exercise service broadcast/exact-answer isolation and instance-local auto-accept/stop reset, using reflected service state rather than starting a valid projection.

## What is replaced or not covered

- `mobile.Node`, `MnetVpnService.currentNode()` and the accessibility entry are explicit test fakes. No Go JNI, active VPN or injected input runs.
- The manifest is disabled. Receiver registration execution is not proof of actual Android signature-permission enforcement.
- No valid projection, captured frame, real consent dialog, actual Activity pixels, device screen content, device/emulator, rotation, reboot or physical network runs.
- SDK simulation is not a declaration that the complete application supports those OS versions.

The Maven AAR runtime extraction in the script is limited to the pinned AndroidX monitor1.8.0 and espresso-idling-resource3.7.0 classes jars required by Robolectric. Main app builds and release outputs are untouched.

## Viewer Activity simulation extension

The actual ViewerActivity/ViewerGate/ViewerTap run with a fake Go SessionViewer. Four extra cases on API24/25/26/33 exercise actual Activity async publication, simulated MotionEvents and destruction. New local assertions use encoded synthetic bitmaps through the real frame callback and require no input before a decoded frame, cancellation across frame resize and no stale dimensions/display after disconnect. API34 Activity creation is excluded because Robolectric4.18 reports a missing VirtualDeviceManager listener class; this is a test-platform gap, not a passing API34 Activity check. Service/encoder tests still include API34. Total49 cases passed904a1af CI. New frame-boundary assertions pass locally, CI pending. No actual UI pixels or accessibility injection are checked.

## Host geometry refusal extension

Two new service tests per simulated API24/25/26/33/34 use reflected capture dimensions/rotation with a fake accessibility sink. They verify no forwarding for uninitialized dimensions, changed width/height/rotation and a latched refusal until a new capture setup. The reset in actual capture setup is source-checked, not executed in these tests.59 total cases pass locally, CI pending. Previous49 passed all-five [2420ae9 CI](https://github.com/MaisySen/Mnet/actions/runs/43922014681). No dynamic rotation, actual accessibility input, valid projection or device execution is proven.
