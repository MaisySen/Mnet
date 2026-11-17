# Android installation and Play Protect

Updated 2026-11-17. Development testing only.

## Current state

The signed 0.8.3-dev/code15 test APK received the Google Play Protect prompt: "App blocked to protect your device" and "This app can request access to sensitive data. This can increase the risk of identity theft or financial fraud." Do not disable protection or keep retrying this APK. Package/signature checks do not establish safety, installability or platform approval.

The compiled APK declares MnetAccessibilityService protected by BIND_ACCESSIBILITY_SERVICE, used for approved hosted-session remote gestures. It declares no READ_SMS, RECEIVE_SMS or notification-listener service. It targets API34 and has valid v2/v3 signing. Google's exact warning guidance describes automatic blocking of internet-sideloaded apps declaring accessibility or specified other sensitive permissions in selected markets. This is the evidence-backed match, not proof of Google's internal classification or a malware finding. Moving the same APK to another web host, renaming it, changing its signing identity, disabling protection or misleading platform declarations are not fixes.

## Source fixes, separate from the delivered APK

The old accessibility XML requested window-content retrieval and window-content-change events, although the implementation ignores accessibility events and uses dispatchGesture. New source sets canRetrieveWindowContent=false and removes event subscription, retaining canPerformGestures=true. The service is not marked as a disability-support accessibility tool.

HostingActivity now shows a dedicated in-app disclosure before opening Accessibility Settings. It explains remote tap/scroll commands, who controls the phone and when, separate screen capture, no accessibility text/window retrieval, how to stop/disable access and what happens on decline. Agree opens Settings; Not now/cancel does not. Other features remain available. Android still requires the user's own enablement decision. Remote control has not been silently removed.

Fresh tests execute actual HostingActivity button/dialog/navigation in simulation on APIs24/25/26/33: 12 cases. Together with the existing 76 normal and 48 VPN cases, 136 pass with no failures/errors/skips. Returning the button to direct Settings causes 12 failures. Compiled Android 34 resources/application Java/minimum API24 dex pass; compiled XML confirms gestures remain and window retrieval is false. These are not a physical install, real Settings enablement or Google review. A simulated dialog render was inspected, not phone pixels.

## Play build readiness, not release approval

Collaborator access to the existing developer account was verified on 2026-11-17. No Mnet Play app, upload, testing release, pricing change or policy/export certification has been created. Account access is not permission to publish.

The private-beta candidate route is internal testing with an explicit tester audience, not public launch or production access. Internal testing can start before complete store setup and supports up to100 testers, but may not receive normal policy/security review. It is not a Play Protect clearance. The12-tester/14-day closed-test requirement concerns production access for qualifying personal accounts, not the first internal test.

Source still targets API34. Current official new-app rules require API36 for phone/tablet submissions since2026-08-31; a personal-account internal test must not be confused with the permanently private organization-app exemption. Target migration needs Android15/16 behavior checks and actual runtime testing before raising the manifest and calling it ready. New source installs shared window-inset handling in MainActivity, HostingActivity and ViewerActivity, preserving base spacing and using the maximum bars/cutout/keyboard extent without padding accumulation.13new simulated cases include actual Main/Hosting wiring;101normal+48VPN cases pass. Removing only the two actual Activity install calls fails8cases; restoring passes. A420x900 native-graphics simulation with synthetic32px status/40px navigation insets was visually inspected. Framework resource strings are stubs in that preview; it is spacing evidence, not a production UI/device screenshot. SDK36 resource/Java/minAPI24dex compilation passes; target remains34.

The older AAR has4KiB ELF LOAD alignment. Current-source rebuilds with explicit max/common-page-size16384 linker flags pass direct program-header checks on all4ABIs. Builders now reject an existing AAR whose native LOAD segments fail16KiB alignment, so a stale library cannot silently pass. Rebuild stale AARs rather than bypassing the gate. `scripts/test-android-native-pages.py` has seven synthetic checks; these checks are layout evidence, not a16KiB-device runtime result. A debug-signed AAB compiles with SDK/build-tools36 using the new AAR while retaining target34. Optional MNET_ANDROID_COMPILE_API and MNET_ANDROID_BUILD_TOOLS select compiler tools, not manifest target. No release binary was uploaded.

Retained accessibility is for remote gestures, not a disability-support tool. Play requires the non-accessibility-tool declaration/disclosure path; VpnService also has its own declaration. Source disclosure changes are not platform approval. Export classification must consider the whole app/dependency sources: public source and standard WireGuard cryptography are relevant evidence, but no blanket compliance certification is made here. Current eCFR742.15(b)(2) limits notification to non-standard cryptography; BIS's broad explanatory page still describes notification without that qualification. The2021FederalRegister amendment explicitly removed standard-crypto public-source/beta-software notification, corroborated by LinuxFoundation guidance, resolving that wording discrepancy. This is not a whole-binary export classification: restricted beta distribution versus the public-object-code provisions, corresponding complete source and applicable exceptions still need review. The740.9(c) beta-test exception has specific importer statements and post-test disposal requirements; none have been imposed on testers or certified. Do not send an unnecessary filing or declare blanket exemption by assumption.

## Supported next steps

1. Finish source/security/privacy review and keep permissions limited to intended functions. These fixes alone may not clear the block because accessibility remains declared.
2. Use an authorized existing Google Play developer account for internal testing with named testers, once account access, declarations, signing and release requirements are met. Google's official internal testing route is Play Store distribution, not an assurance of review or approval. No new developer-account signup/payment or release was performed during this investigation.
3. If the reviewed app is still believed to be incorrectly blocked, use Google's Play Protect appeal process. The currently fetched appeal form requires the SHA256 of an APK uploaded to VirusTotal. Uploading a private test artifact to an additional service needs explicit disclosure approval. No such upload or appeal was submitted.

## Sources

- [Google warning-specific developer guidance](https://developers.google.com/android/play-protect/warning-dev-guidance): exact warning, sensitive-permission/distribution explanation and review-before-appeal guidance.
- [AccessibilityServiceInfo API](https://developer.android.com/reference/android/accessibilityservice/AccessibilityServiceInfo): separate gesture/window-content capabilities and event selection.
- [Accessibility API policy/declaration](https://support.google.com/googleplay/android-developer/answer/10964491): non-disability-tool disclosure, affirmative consent and declaration requirements.
- [Sensitive APIs policy](https://support.google.com/googleplay/android-developer/answer/16558241?hl=en): narrow permissions, no security-control bypass, consent and VPN requirements.
- [Prominent disclosure guidance](https://support.google.com/googleplay/android-developer/answer/11150561): separate in-app just-in-time disclosure and decline behavior.
- [Official internal testing](https://support.google.com/googleplay/android-developer/answer/9845334?hl=en): named testers and secure Play Store distribution; internal testing may not receive normal policy/security review.
- [Play Protect appeal form](https://support.google.com/googleplay/android-developer/contact/protectappeals): current VirusTotal-uploaded APK hash field. No submission performed.

- [Current target API rules](https://support.google.com/googleplay/android-developer/answer/11926878?hl=en): API36 new-app requirement; organization-private exception is distinct from internal testing.
- [Personal account testing](https://support.google.com/googleplay/android-developer/answer/14151465?hl=en): production closed-test requirements.
- [Android16 target behavior](https://developer.android.com/about/versions/16/behavior-changes-16): edge-to-edge and adaptive layouts.
- [16KiB native support](https://developer.android.com/guide/practices/page-sizes): ELF/build requirements, separate from runtime testing.
- [Export guidance](https://support.google.com/googleplay/android-developer/answer/113770?hl=en): encryption evaluation required even for open source.
- [Current742.15](https://www.ecfr.gov/current/title-15/subtitle-B/chapter-VII/subchapter-C/part-742/section-742.15) and [734.3](https://www.ecfr.gov/current/title-15/subtitle-B/chapter-VII/subchapter-C/part-734/section-734.3): public-source/object-code provisions and qualifications.
- [BIS explanatory page](https://media.bis.gov/learn-support/encryption-controls/encryption-items-not-subject-to-ear): broad notification wording differs from current regulation qualification.

- [2021FederalRegister amendment](https://www.federalregister.gov/documents/2021/03/29/2021-05481/wassenaar-arrangement-2019-plenary-decisions-implementation-of-other-changes-to-the-commerce-control): standard-crypto public-source/beta notification removed.
- [LinuxFoundation interpretation](https://bestpractices.linuxfoundation.org/regulatory/export/non-standard-cryptography.html): confirms current source/crypto conditions, not a Mnet-specific legal opinion.
- [Beta-test exception conditions](https://www.ecfr.gov/current/title-15/subtitle-B/chapter-VII/subchapter-C/part-740/section-740.9): importer statement, transfer restriction and post-test disposal; no tester commitment made.
