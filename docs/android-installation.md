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

The latest target36/minimum24 candidate has fresh16KiB-aligned4ABI bindings, separate VPN data disclosure and actual API34/36 full traffic/consent/capture/control/reconnect tests. It is separate from the earlier blocked target34 APK. See [current status](STATUS.md) and [submission evidence](android-submission-evidence.md) for exact candidate and run links.

Mnet is Free in the intended existing account. The owner manually uploaded the preserved signed target36 code15 AAB, then the restricted internal release was published at22:27 Australia/Sydney on2026-11-17. Console shows Active and Available to internal testers. Only the approved single-owner list is selected. The owner received the test link privately; it resolves to Google's testing page, but the collaborator account is correctly not invited. Owner opt-in, installation and Play Protect behavior remain unverified. The temporary store name is au.fintex.mnet (unreviewed). The AAB cannot be directly installed on a phone.

Play automatically provisioned an app-signing key different from the old sideload signer; do not promise an in-place upgrade. Do not disable install protection, rename artifacts to evade it, misstate accessibility usage or upload a private artifact to VirusTotal. The appeal route remains on hold.

Internal testing may begin before full store setup, but is not normal policy/security approval. The12-tester/14-day production-access rule is separate. Operator and narrow Australian private-sector internal-development facts have been confirmed for this owner-only plan; preserve that boundary rather than infer a general export clearance. Full production setup, exact provider-log duration and physical/OEM/soak tests are not automatically owner-test upload gates. Actual Console validation and truthful required certifications still matter.

## Supported next steps

1. Finish source/security/privacy review and keep permissions limited to intended functions. These fixes alone may not clear the block because accessibility remains declared.
2. Use an authorized existing Google Play developer account for internal testing with named testers, once account access, declarations, signing and release requirements are met. Google's official internal testing route is Play Store distribution, not an assurance of review or approval. The restricted internal release is now published; no new developer-account signup/payment, public launch or broader tester access was performed.
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
