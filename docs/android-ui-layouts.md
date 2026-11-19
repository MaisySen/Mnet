# Android screen layout checkpoints

19 November 2026. Implemented application screens, not a separate web mockup.

## Journey and scope

The home screen puts connection state first, then distinct actions to view another device and share this screen. Manual connection settings are available in an expandable section. Multies sign-in is explicitly unavailable in this development build. This work does not supply enrollment, a workspace device registry, entitlement or a new integration policy.

On an empty form, the primary action reveals the actual settings and a required-field error. It does not save settings, mark the disclosure accepted, request VPN permission or start a service. After required details are present, the existing disclosure and Android VPN consent still apply. Auth-key text is masked. Field labels persist above entered values.

The sharing screen separates viewing from optional remote control and Accessibility. The existing view-only auto-accept option remains off by default and applies to the next hosting start only. Existing exact-request consent and control decisions are unchanged. Start/Stop availability follows service-owned running state. An app-local state notification asks the UI to re-read that state; log messages do not establish whether sharing is active. Stop hosting stays in a fixed footer while the rest scrolls.

## Viewer follow-up

The actual viewer now uses the same native screen/card/button styling. It has explicit connecting, waiting-for-screen, viewing, interrupted and missing-network states, a dark image canvas, an optional expandable chat section and a fixed End viewing footer. Control-request/cancel/allowed/paused/declined/revoked/unavailable states still come through the existing session/control gate. The host remains the control authority; a new appearance does not grant input.

The image canvas retains FIT_CENTER mapping and frame-readiness/gesture gating. A framework regression dispatches a completed tap and a multi-move scroll through the actual nested root: the tap sends one fake remote click, scrolling moves the local viewport without becoming remote input. Existing resize/cancel/revocation/late-publication tests remain passing. The first new scroll fixture contained only one move, which initiated parent interception but did not produce a subsequent drag delta; a realistic multi-move sequence passes. That failed fixture is not a passing scroll result.

The exit-node hint is shortened to an example; its persistent label keeps the meaning. No exit-node policy changes.

## Inspected pixels

These are native Android View rasterizations on Robolectric API33 using actual production Activity/helper source and aapt2-packaged application resources. Go, VPN, Accessibility and service/peer state are isolated test doubles or explicit fixture state. The capture draws the actual scroll content at its measured viewport offset and the actual fixed footer. It does not draw Android system bars, a physical display or a real remote screen.

- [Home,390x844,100% text](ui/android/390-home.png)
- [Inactive sharing,390x844](ui/android/390-host.png)
- [Active sharing fixture,390x844](ui/android/390-host-active.png)
- [Setup bottom,320x720,160% text](ui/android/320-large-settings-bottom.png)
- [Sharing bottom,320x720,160% text](ui/android/320-large-host-bottom.png)
- [Error details,740x360 landscape](ui/android/740-land-error-details.png)
- [Viewer waiting for a screen](ui/android/390-viewer-waiting.png)
- [Viewer pending control](ui/android/390-viewer-pending.png)
- [Viewer allowed control](ui/android/390-viewer-allowed.png)
- [Viewer revoked control,320dp/160% text](ui/android/320-large-viewer-revoked.png)
- [Viewer chat lower controls,320dp/160% text](ui/android/320-large-viewer-chat.png)
- [Viewer without a network connection](ui/android/390-viewer-no-network.png)
- [Viewer paused control,landscape](ui/android/740-land-viewer-paused.png)

Viewer images that contain a picture use an explicitly labelled synthetic test bitmap, not a remote device or captured app. These images do not establish real screen quality, input precision or a working remote session.

Inspected hierarchy, wrapping, field labels, active/disabled states, lower controls and footer placement. The compact enlarged-text tests measure content larger than its viewport, scroll to its bottom and assert lower controls remain reachable. Single-line inputs retain horizontal editing instead of wrapping secrets/URLs across the form. Insets are covered separately by simulated framework tests; these images are not physical edge-to-edge, keyboard, TalkBack, color-calibration or OS permission acceptance.

## Reproduce

With JDK17, Maven, Python3 and Android SDK36/build-tools36.0.0:

```sh
ANDROID_HOME=/path/to/android-sdk MNET_LAYOUT_OUTPUT=/absolute/output sh scripts/render-android-layouts.sh
sh scripts/test-android-framework.sh
sh scripts/test-android-framework.sh vpn
```

`MVN` may select the Maven executable and `MNET_MAVEN_REPO` its dependency cache. The layout script builds a fresh isolated project with generated app resources, runs nine fixtures and emits57PNG views of home/setup/approval/managed/error/hosting/viewer states. It creates no APK, installs nothing and does not start a real VPN or contact the example device URL. The functional framework suite continues to use placeholder resources; only the layout suite packages the actual app resources.

At the viewer checkpoint, fresh local results are168 selected Activity/service/helper cases,78 VPN-service cases and nine layout fixtures, all passing. Full production Java compilation, resource packaging and min-API24 dex compilation also pass against a newly built current-Go four-ABI AAR, without APK signing or installation. Local API/always-on/projection/viewer/control-safety/log checks pass. These are not new emulator, physical-device, JNI, screen capture, control or release results.

Initial fixture setup failures were corrected before recording the passing evidence: invalid resource-qualifier order, a ServiceController used as AutoCloseable, scroll capture/focus ordering, and missing main-loop draining for the peer dialog on-show listener. An attempted application compile first used the checkout's stale AAR, which lacked already-existing control symbols; fresh current-Go bindings fixed the mismatch without an engine change. No failed attempt is counted as acceptance.

Normal general CI now runs the isolated actual-resource layout suite and retains only the synthetic PNG output. Passing render tests establish layout generation and measured geometry, not human visual acceptance. The linked images were inspected separately. The previously closed combined native acceptance workflow remains outside this work and is cancelled when app-path pushes trigger it.

## Remaining product work

iOS and desktop UI are separate remaining lanes. Manual server/network/key setup is an honest development limitation, not the intended completed Multies journey. Workspace integration stays deferred. Native device acceptance, signing, deployment, publication, retention and other policy decisions remain outside this checkpoint.
