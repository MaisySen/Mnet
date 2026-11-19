# Android home, setup and sharing layout checkpoint

19 November 2026. Implemented application screens, not a separate web mockup.

## Journey and scope

The home screen puts connection state first, then distinct actions to view another device and share this screen. Manual connection settings are available in an expandable section. Multies sign-in is explicitly unavailable in this development build. This work does not supply enrollment, a workspace device registry, entitlement or a new integration policy.

On an empty form, the primary action reveals the actual settings and a required-field error. It does not save settings, mark the disclosure accepted, request VPN permission or start a service. After required details are present, the existing disclosure and Android VPN consent still apply. Auth-key text is masked. Field labels persist above entered values.

The sharing screen separates viewing from optional remote control and Accessibility. The existing view-only auto-accept option remains off by default and applies to the next hosting start only. Existing exact-request consent and control decisions are unchanged. Start/Stop availability follows service-owned running state. An app-local state notification asks the UI to re-read that state; log messages do not establish whether sharing is active. Stop hosting stays in a fixed footer while the rest scrolls.

## Inspected pixels

These are native Android View rasterizations on Robolectric API33 using actual production Activity/helper source and aapt2-packaged application resources. Go, VPN, Accessibility and service/peer state are isolated test doubles or explicit fixture state. The capture draws the actual scroll content at its measured viewport offset and the actual fixed footer. It does not draw Android system bars, a physical display or a real remote screen.

- [Home,390x844,100% text](ui/android/390-home.png)
- [Inactive sharing,390x844](ui/android/390-host.png)
- [Active sharing fixture,390x844](ui/android/390-host-active.png)
- [Setup bottom,320x720,160% text](ui/android/320-large-settings-bottom.png)
- [Sharing bottom,320x720,160% text](ui/android/320-large-host-bottom.png)
- [Error details,740x360 landscape](ui/android/740-land-error-details.png)

Inspected hierarchy, wrapping, field labels, active/disabled states, lower controls and footer placement. The compact enlarged-text tests measure content larger than its viewport, scroll to its bottom and assert lower controls remain reachable. Single-line inputs retain horizontal editing instead of wrapping secrets/URLs across the form. Insets are covered separately by simulated framework tests; these images are not physical edge-to-edge, keyboard, TalkBack, color-calibration or OS permission acceptance.

## Reproduce

With JDK17, Maven, Python3 and Android SDK36/build-tools36.0.0:

```sh
ANDROID_HOME=/path/to/android-sdk MNET_LAYOUT_OUTPUT=/absolute/output sh scripts/render-android-layouts.sh
sh scripts/test-android-framework.sh
sh scripts/test-android-framework.sh vpn
```

`MVN` may select the Maven executable and `MNET_MAVEN_REPO` its dependency cache. The layout script builds a fresh isolated project with generated app resources, runs five fixtures and emits23PNG views of home/setup/approval/managed/error/hosting states. It creates no APK, installs nothing and does not start a real VPN or contact the example device URL. The functional framework suite continues to use placeholder resources; only the layout suite packages the actual app resources.

At this checkpoint, fresh local results are164 selected Activity/service/helper cases,78 VPN-service cases and five layout fixtures, all passing. Full production Java compilation, resource packaging and min-API24 dex compilation also pass against a newly built current-Go four-ABI AAR, without APK signing or installation. Local API/always-on/projection/viewer/control-safety/log checks pass. These are not new emulator, physical-device, JNI, screen capture, control or release results.

Initial fixture setup failures were corrected before recording the passing evidence: invalid resource-qualifier order, a ServiceController used as AutoCloseable, scroll capture/focus ordering, and missing main-loop draining for the peer dialog on-show listener. An attempted application compile first used the checkout's stale AAR, which lacked already-existing control symbols; fresh current-Go bindings fixed the mismatch without an engine change. No failed attempt is counted as acceptance.

## Remaining product work

The viewer layout still needs the same screen hierarchy and waiting/error/control treatment. iOS and desktop UI are separate remaining lanes. Manual server/network/key setup is an honest development limitation, not the intended completed Multies journey. Workspace integration stays deferred. Native device acceptance, signing, deployment, publication, retention and other policy decisions remain outside this checkpoint.
