# Google Play internal beta readiness

Updated 2026-11-17. This is an engineering and submission-preparation checklist, not a compliance certification or permission to publish. Keep account identifiers, tester addresses, credentials and private artifacts out of this document.

## Intended route

Mnet is Free in the intended existing account. The preserved signed code15 bundle was manually uploaded by the owner, then validated and published to the restricted internal track at22:27 Australia/Sydney on2026-11-17. Console shows Active and Available to internal testers. Only the single approved owner list is selected, containing one account. The test link was delivered privately; the invited account's opt-in and installation remain unverified. No service-account key, alternate account or API-write workaround was used.

Internal testing can begin before full store setup and supports up to100 testers, but this plan authorizes only one. It may not receive ordinary policy/security review and is not proof of Play Protect clearance. The12-testers/14-days closed-test rule concerns production access for qualifying personal accounts, not this initial internal test.

## Owner-internal upload/release path

The immediate path is supported Create app certifications, a valid upload-signed bundle, Play App Signing, the single approved internal tester and the actual console release validation. Internal testing can start before full app setup. A production listing, production-access closed-test requirements, physical/OEM/soak completion and a precisely known provider-log duration are not automatically owner-test upload gates. Those wider checks continue independently. A real console validation error or unsupported factual certification remains a gate.

The published Android internal-test privacy policy is https://maisysen.github.io/Mnet/ . It states actual handling and manual-request/provider-retention limits without inventing a duration.

## Evidence and remaining work

| Gate | Verified state | Remaining work |
| --- | --- | --- |
| Account/app | Mnet exists as Free; code15 restricted internal release is active | Do not change unrelated apps or expand audience |
| Candidate | Upload-signed0.8.3-dev/code15 AAB validates; SHA256 b8abf192457c4a02bd652e347024d764869ae1fb6aaa3105afe7c34d436aeae3; all4ABIs have16KiB ELF LOAD alignment | Play processed the upload and reports16KB support; this is not16KiB-device runtime proof |
| Signing | Automatic Play app-signing key differs from earlier sideload signer | Do not change keys or promise in-place sideload upgrade |
| Runtime | Actual target36 API34/36 matrices pass at8def887 and2ddb11e, including independent-app traffic, reconnect, capture/control and stop | Earlier intermittent failures, physical/OEM/radio/reboot/soak remain open; some screenshot frames are not final visual proof |
| VPN/privacy | Separate first-connect disclosure, decline/save refusal and background service acceptance gate; published policy and explicit in-app link | Apply truthful declarations if Console requires them; no exact provider-log duration invented |
| Export | Owner confirmed bounded Australian private-sector internal-development facts; narrow route assessed before Create app | Preserve owner-only purpose and restrictions; not general worldwide clearance |
| Audience | Only the approved owner list with one account is saved and selected | No additional testers or public launch |
| Upload/release | Owner upload completed; Console validation passed with two optional symbol/deobfuscation warnings; restricted release available to internal testers | Owner-account opt-in/install, propagation and physical-device behavior remain unverified |

## Source-backed disclosure inventory

This inventory guides review; it is not a completed Data safety form. Internal-only tests are exempt from the Data safety section under current help, but privacy obligations and truthful app declarations remain.

- `android/.../MainActivity.java` stores control server, network, enrollment key, device name and optional exit-node choice in private SharedPreferences. `MnetVpnService.java` reads these for connection. Private app storage is not a claim of hardware encryption or absence of data.
- `mobile/mobile.go` and `internal/daemon/state.go` store private node key/join identity under the app-private state directory. Never publish those files or logs containing them.
- The control plane handles device names, public keys, assigned and underlay IPs/endpoints, node credentials, approval and network topology. OIDC paths can handle email, subject and display name. Review `internal/control/store.go`, `oidc.go` and actual operator configuration.
- NAT traversal sends STUN binding requests to configured public Google/Cloudflare STUN servers. Those providers can observe source IP/port and discovery metadata; they are additional data recipients, not hidden by mesh payload encryption.
- WireGuard encrypts payloads between endpoints, including when relayed. The operator/relay can still observe traffic and connection metadata. Do not claim that encryption hides all metadata or protects a compromised endpoint.
- Hosted sessions transmit screen frames and accept remote gestures after the applicable permissions/consent. Viewer endpoints can see shared content. Session chat/files and optional recording/storage differ by platform; verify enabled Android code paths before making any universal retention claim.
- The AccessibilityService is not a disability-support tool. It ignores accessibility events/window contents and dispatches gestures. Screen frames come through separate MediaProjection consent, not accessibility text extraction. These distinctions must remain clear in listing, disclosure, video and declarations.

The detailed [Android privacy evidence](android-privacy-evidence.md) separates current platform data paths, new backup exclusions, operator retention/deletion gaps and unverified features.

See the [submission evidence pack](android-submission-evidence.md) for current runtime/signing results, draft declaration basis and the restricted-binary export decision table.

## Declaration preparation

Before Create app, the console requires a Developer Program Policies confirmation and an export-law certification, including software with encryption functions. Do not treat draft creation as legally empty preparation. Record a supported factual basis first.

For accessibility, use the non-accessibility-tool route, describe actual remote-control gestures, separate disclosure and decline effects. Do not set the disability-tool flag to avoid review. For VpnService, describe the mesh VPN/network tool purpose and actual encryption path. A demonstration video must show the real app flow and complete disclosure, not a simulation presented as device evidence.

Export research established that the2021 rule removed notification for standard-crypto public-source/beta software, with non-standard cryptography retaining notification. Public-source availability alone does not answer every restricted-binary question. The alternative740.9(c) beta exception has importer statements, transfer restrictions and post-test disposal conditions. No filing, tester obligation or final legal classification has been made.

## Wider validation work, alongside the internal release

1. Repeat final target36 candidate runtime proof, including consent/capture/input. Preserve the earlier intermittent TCP failure and physical-device gaps rather than treating one green matrix as universal reliability.
2. Finish final-binary data-flow/policy/export evidence, privacy text and declaration/video drafts. Escalate only an unresolved substantive certification, not routine preparation.
3. Free pricing and the exact sole tester are already resolved. Do not ask them again.
4. Create the intended app draft only with supported declarations. Inspect signing/enrollment and final AAB before upload.
5. Configure only the approved internal tester audience and release. Verify final console state and tester link without claiming production publication or security clearance.

## Current sources

- https://support.google.com/googleplay/android-developer/answer/9845334?hl=en
- https://support.google.com/googleplay/android-developer/answer/14151465?hl=en
- https://support.google.com/googleplay/android-developer/answer/11926878?hl=en
- https://support.google.com/googleplay/android-developer/answer/10964491?hl=en
- https://support.google.com/googleplay/android-developer/answer/12564964
- https://support.google.com/googleplay/android-developer/answer/113770?hl=en
- https://developer.android.com/guide/practices/page-sizes
- https://www.ecfr.gov/current/title-15/subtitle-B/chapter-VII/subchapter-C/part-742/section-742.15
- https://www.ecfr.gov/current/title-15/subtitle-B/chapter-VII/subchapter-C/part-734/section-734.7
- https://www.ecfr.gov/current/title-15/subtitle-B/chapter-VII/subchapter-C/part-740/section-740.9
- https://www.federalregister.gov/documents/2021/03/29/2021-05481/wassenaar-arrangement-2019-plenary-decisions-implementation-of-other-changes-to-the-commerce-control
- https://bestpractices.linuxfoundation.org/regulatory/export/non-standard-cryptography.html
