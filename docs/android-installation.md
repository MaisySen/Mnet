# Android installation and Play Protect

Updated 2026-11-17. Development testing only.

## Current state

The signed 0.8.3-dev/code15 test APK received the Google Play Protect prompt: "App blocked to protect your device" and "This app can request access to sensitive data. This can increase the risk of identity theft or financial fraud." Do not disable protection or keep retrying this APK. Package/signature checks do not establish safety, installability or platform approval.

The compiled APK declares MnetAccessibilityService protected by BIND_ACCESSIBILITY_SERVICE, used for approved hosted-session remote gestures. It declares no READ_SMS, RECEIVE_SMS or notification-listener service. It targets API34 and has valid v2/v3 signing. Google's exact warning guidance describes automatic blocking of internet-sideloaded apps declaring accessibility or specified other sensitive permissions in selected markets. This is the evidence-backed match, not proof of Google's internal classification or a malware finding. Moving the same APK to another web host, renaming it, changing its signing identity, disabling protection or misleading platform declarations are not fixes.

## Source fixes, separate from the delivered APK

The old accessibility XML requested window-content retrieval and window-content-change events, although the implementation ignores accessibility events and uses dispatchGesture. New source sets canRetrieveWindowContent=false and removes event subscription, retaining canPerformGestures=true. The service is not marked as a disability-support accessibility tool.

HostingActivity now shows a dedicated in-app disclosure before opening Accessibility Settings. It explains remote tap/scroll commands, who controls the phone and when, separate screen capture, no accessibility text/window retrieval, how to stop/disable access and what happens on decline. Agree opens Settings; Not now/cancel does not. Other features remain available. Android still requires the user's own enablement decision. Remote control has not been silently removed.

Fresh tests execute actual HostingActivity button/dialog/navigation in simulation on APIs24/25/26/33: 12 cases. Together with the existing 76 normal and 48 VPN cases, 136 pass with no failures/errors/skips. Returning the button to direct Settings causes 12 failures. Compiled Android 34 resources/application Java/minimum API24 dex pass; compiled XML confirms gestures remain and window retrieval is false. These are not a physical install, real Settings enablement or Google review. A simulated dialog render was inspected, not phone pixels.

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
