# Disposable Android emulator integration

This is a separate engineering test, not a store build, private-user APK, Play Protect verdict or physical-device proof. It never disables Android's install protection or changes a personal device. The production source keeps its normal network/security manifest.

## Scope and safeguards

The standard `ubuntu-latest` public CI runner boots separate Android34 and Android36 Google APIs x86_64 emulators for the target36 candidate using KVM. It builds current Go bindings and the actual app from a fresh source archive. Only the throwaway copy is marked debuggable and allowed cleartext loopback, for its local control-plane fixture. It uses a disposable debug signer, not an upload/release credential. No test APK is retained as an artifact or sent to a user.

A real Go userspace mesh peer serves a bidirectional4MiB SHA256 protocol on its mesh IP, not on the host network. Instrumentation launches actual MainActivity, fills only fixture credentials, clicks the OS VPN consent dialog in the fresh emulator, waits for actual Node up, verifies both payload directions, manually stops and reconnects after the worker exits, then verifies both directions again and stops. It does not mock the Go node or Android VPN interface.

Expected outputs are explicit PASS summary and a pre-credential launch screenshot. The public artifact allowlist contains only summary.json, instrumentation.txt and launch.png. State, enrollment keys, APKs, raw instrumentation output and logcat are not uploaded. Fixture control listens on loopback, Android reaches it via adb reverse, and temporary fixture files/processes are removed. No existing service/network/account is touched, and no paid/larger runner is selected.

## Current evidence

Instrumentation compiles against the actual app classes/Android34 SDK and dexes at minimumAPI24. Python/YAML parsing and eight fixture guard tests pass locally. No emulator is installed locally and KVM is unavailable there. The first CI run failed during toolchain setup with exit127, before building or booting: the bare SDK CLI command was the suspected cause (full first-run logs were not available). The follow-up uses explicit installed SDK CLI paths. The follow-up completed SDK/KVM/shared-engine setup but failed at emulator boot, before instrumentation. Fixed diagnostics confirmed the emulator exited before boot, without a match in the first error categories. Follow-up expands fixed categories and records numeric exit code/free-space only, uses current swiftshader mode instead of its deprecated alias and bounds the fixture data partition to2048MiB. Later fixed diagnostics isolated unknown/missing AVD name with exit1 and81591MiB free, not disk pressure. Workflow now sets one explicit disposable ANDROID_AVD_HOME for creation/boot, an explicit AVD path, and verifies its ini/listing before engine compilation. GPU/storage changes were setup remediations, not the verified cause. Raw emulator logs are never uploaded. Explicit AVD setup then booted successfully, installed the real APK, launched JNI, clicked actual OS VPN consent and reached node-up. The initial real mesh TCP connects timed out despite handshake and transport traffic. Native diagnostics showed TUN reads but no inbound writes/errors. The4f888b5 run then passed actual bidirectional4MiB hash, stop/reconnect and repeat hash: https://github.com/MaisySen/Mnet/actions/runs/44018146248 . No packet-semantic change explains that pass, so it is one successful run rather than a proven reliability fix. Its pre-key launch image was blank because capture preceded first draw; it is not layout evidence. A first-layout gate and repeat run followed. The new target36 candidate has local SDK36 build/compile and149framework simulation checks; its actual API34/36 matrix at01e2b36 passed both, but the next780 repeat passed API34 and failed API36 TCP connect. Wrong-source diagnostics persist despite waiting for OS VPN publication. Explicit local binding is now an isolated diagnostic, not ordinary unbound-client proof. API34 screenshot pixels are readable; API36 screenshot was still blank, so its visual evidence remains open. Instrumentation now distinguishes TCP connect/send/hash/reverse stages and exports only boolean peer/handshake/traffic diagnostics, not names, addresses, keys or status JSON.

## Limits and next stages

- This first stage does not yet execute remote screen capture/input, real system accessibility enablement, network-radio loss, reboot/unlock/always-on or long-session soak.
- A real peer plus emulator is not two physical phones, a public-internet test or a manufacturer battery-policy test.
- Google APIs engineering images and adb instrumentation do not test the Google Play installation/approval path. The private-user APK's Play Protect block remains a separate issue.
- A stage that times out/fails is a failure to diagnose, never partial evidence of an end-to-end pass. No release or physical-phone retry follows automatically.

## References

- https://github.blog/changelog/2024-04-02-github-actions-hardware-accelerated-android-virtualization-now-available/
- https://developer.android.com/reference/android/app/Instrumentation
- https://developer.android.com/reference/android/app/UiAutomation
