# Selected Android framework simulation

## Run

Run `sh scripts/test-android-framework.sh` with JDK17, Python3, Maven and network access to Maven Central/Google Maven. `MVN` can name the Maven executable; `MNET_MAVEN_REPO` selects the dependency cache. The script copies current production sources into a fresh temporary project, resolves pinned dependencies, runs tests and removes the temporary project.

CI caches Maven dependencies using the framework POM as its dependency key, reducing repeated downloads. A cache does not skip compilation/tests, change pinned versions or make a network failure a pass.

The harness uses Robolectric4.18. Its AAR extraction is limited to pinned AndroidX monitor1.8.0 and espresso-idling-resource3.7.0 classes jars. It does not change application builds or release outputs.

## Current coverage

64 cases pass in a fresh local run at `d0afcb6`. All five jobs passed in [that revision's CI](https://github.com/MaisySen/Mnet/actions/runs/43994622318).

| Actual application source | Simulated API levels | Cases | Checks |
| --- | --- | --- | --- |
| HostSessionService, SessionConsent, ProjectionGuard, VpnPlatform, CaptureEncoder |24,25,26,33,34|60 (12 per API)|Notification/channel construction, receiver registration, nonsticky restart/stop, missing-consent cleanup, blocking consent cancellation/exact-answer isolation, instance-local auto-accept/reset, synthetic RGBA-to-JPEG row bounds, host input geometry refusal and pointer validation|
| ViewerActivity, ViewerGate, ViewerTap |24,25,26,33|4 (one per API)|Actual asynchronous connection publication, simulated down/up/cancel/drag dispatch, synthetic JPEG callback, no input before a decoded frame, resize cancellation, disconnect clearing and destruction|

Service tests use reflection to set capture/running state rather than start a valid projection. The geometry-refusal latch resets in actual capture setup according to source checks; that restart path is not executed here. Pointer checks preserve omitted-zero coordinates and reject malformed/unsupported/out-of-frame values.

## Explicit replacements

`mobile.Node`, `SessionViewer`, `MnetVpnService.currentNode()` and the accessibility endpoint are test fakes. No Go JNI, real VPN or injected accessibility gesture executes. Viewer frame bytes are synthetic JPEGs, not device screen content. Encoder inputs are synthetic RGBA buffers, not an actual ImageReader. The manifest is disabled, so receiver registration execution is not proof of signature-permission enforcement.

## Known gaps

- API34 Activity creation reports a missing VirtualDeviceManager listener class in Robolectric4.18. Activity tests explicitly omit API34; service/encoder tests retain it.
- No valid projection, real consent dialog, physical display rotation, actual Android UI pixels, device/emulator, reboot or physical network runs.
- Real accessibility finite checks and removal of down/up-as-click have source/build checks only, not runtime dispatch proof.
- Synthetic JPEG decoding confirms code paths and dimensions, not device color, orientation, heap pressure or playback appearance.
- Simulated API coverage does not declare complete application support for those OS versions.

## Build evidence is separate

At `d0afcb6`, a fresh local current-Go Android AAR built with minimum API24 for armeabi-v7a, arm64-v8a, x86 and x86_64. Full Android34 resource generation/application Java and min-api24 dex compilation against that AAR passed. These build checks do not execute JNI or validate a signed APK, installation or device behavior. No new APK/AAB or release was published.
