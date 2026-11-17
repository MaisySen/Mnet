# Disposable Android emulator integration

This is a separate engineering test, not a store build, private-user APK, Play Protect verdict or physical-device proof. It never disables Android's install protection or changes a personal device. The production source keeps its normal network/security manifest.

## Scope and safeguards

The standard `ubuntu-latest` public CI runner boots an Android34 Google APIs x86_64 emulator using KVM. It builds current Go bindings and the actual app from a fresh source archive. Only the throwaway copy is marked debuggable and allowed cleartext loopback, for its local control-plane fixture. It uses a disposable debug signer, not an upload/release credential. No test APK is retained as an artifact or sent to a user.

A real Go userspace mesh peer serves a bidirectional4MiB SHA256 protocol on its mesh IP, not on the host network. Instrumentation launches actual MainActivity, fills only fixture credentials, clicks the OS VPN consent dialog in the fresh emulator, waits for actual Node up, verifies both payload directions, manually stops and reconnects after the worker exits, then verifies both directions again and stops. It does not mock the Go node or Android VPN interface.

Expected outputs are explicit PASS summary and a pre-credential launch screenshot. The public artifact allowlist contains only summary.json, instrumentation.txt and launch.png. State, enrollment keys, APKs, raw instrumentation output and logcat are not uploaded. Fixture control listens on loopback, Android reaches it via adb reverse, and temporary fixture files/processes are removed. No existing service/network/account is touched, and no paid/larger runner is selected.

## Current evidence

Instrumentation compiles against the actual app classes/Android34 SDK and dexes at minimumAPI24. Python/YAML parsing and six fixture guard tests pass locally. No emulator is installed locally and KVM is unavailable there. The first CI run failed during toolchain setup with exit127, before building or booting: the runner did not resolve the bare SDK CLI command. The follow-up uses explicit installed SDK CLI paths. No Android emulator integration pass is claimed yet.

## Limits and next stages

- This first stage does not yet execute remote screen capture/input, real system accessibility enablement, network-radio loss, reboot/unlock/always-on or long-session soak.
- A real peer plus emulator is not two physical phones, a public-internet test or a manufacturer battery-policy test.
- Google APIs engineering images and adb instrumentation do not test the Google Play installation/approval path. The private-user APK's Play Protect block remains a separate issue.
- A stage that times out/fails is a failure to diagnose, never partial evidence of an end-to-end pass. No release or physical-phone retry follows automatically.

## References

- https://github.blog/changelog/2024-04-02-github-actions-hardware-accelerated-android-virtualization-now-available/
- https://developer.android.com/reference/android/app/Instrumentation
- https://developer.android.com/reference/android/app/UiAutomation
