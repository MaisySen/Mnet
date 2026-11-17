# Google Play internal beta readiness

Updated 2026-11-17. This is an engineering and submission-preparation checklist, not a compliance certification or permission to publish. Keep account identifiers, tester addresses, credentials and private artifacts out of this document.

## Intended route

Use the existing developer account and a named-audience Internal testing track for the first phone beta. Collaborator access has been verified. No Mnet app record, binary upload, testing release, public launch or pricing change has been made. Internal testing can begin before full store setup and supports up to100 testers. It may not receive ordinary policy/security review and is not proof of Play Protect clearance. The12-testers/14-days closed-test rule concerns production access for qualifying personal accounts, not this initial internal test.

## Release gates

| Gate | Current evidence | Remaining work |
| --- | --- | --- |
| Account access | Intended existing developer account opens with invited collaborator | Confirm app-specific permissions when a draft exists; do not modify unrelated apps |
| Current Android target | Source candidate now targets36; SDK/build-tools36 compilation works | API34/36 actual candidate matrix pending; validate Android15/16 behavior, real consent, stop/reconnect and layouts |
| Native16KiB support | Fresh current-source AAR4ABIs pass direct ELF LOAD checks; old AAR rejected | Actual16KiB-device/runtime check, not only ELF structure |
| App Bundle | Debug AAB generation and bundletool validation pass | Fresh final-source AAR/AAB, correct version, existing upload identity; review Play App Signing choice in actual flow |
| Actual runtime | Emulator boots, installs actual APK/JNI, performs OS VPN consent and reaches node-up | One actual API34 emulator run at4f888b5 passes bidirectional4MiB hash, stop/reconnect and repeat. Earlier runs timed out at TCP connect; no packet-semantic fix explains the later pass. The target36 API34/36 runs also time out; diagnostic TCP source differs from assigned self while destination matches peer. OS VPN-publication wait produced one full API34/36 pass at01e2b36; next780 repeat passes API34 but fails API36. Explicit local-mesh socket binding passes two repeated API34/36 matrices. Mnet mobile viewer source now selects its assigned local IP; independent-app unbound TCP proof is added and pending after correcting test APK permission/process-lifetime errors. Two corrected runs at669e7d3/d5bee99 now pass independent unbound app traffic on both API34/36, plus self-bound reconnect tests. Physical/OEM and screen/control proof remain separate. No production packet-semantic fix is established. Linux exact mobile-fd/WireGuard/TCP regression also passes, but is not Android proof |
| Screen control | Gesture feature retained; unused window retrieval/events removed; dedicated disclosure. Actual API34/36 MediaProjection/viewer consent, full JPEG decoding at peer and stop-closure pass ate433269 | API36 repeat and actual input proof, non-tool accessibility declaration and supporting consent video |
| VPN policy | Mesh networking/remote access matches a listed permitted purpose in policy | Verify precise listing, device-to-endpoint encryption, data disclosures and declaration against final binary |
| Privacy | Source has real metadata and user-content flows, not a zero-data claim | Final inventory, operator retention/deletion route, privacy-policy destination and truthful declaration answers |
| Export | Standard-crypto public-source notification rule researched | Whole-binary restricted-beta route and certification basis still unresolved; do not manufacture compliance |
| Commercial direction | Not selected | User owns permanent Free/Paid direction; free cannot become paid after publication |
| Audience | Not selected | Ground exact tester Google account(s); an earlier build-email recipient is not a standing broader tester grant |

## Source-backed disclosure inventory

This inventory guides review; it is not a completed Data safety form. Internal-only tests are exempt from the Data safety section under current help, but privacy obligations and truthful app declarations remain.

- `android/.../MainActivity.java` stores control server, network, enrollment key, device name and optional exit-node choice in private SharedPreferences. `MnetVpnService.java` reads these for connection. Private app storage is not a claim of hardware encryption or absence of data.
- `mobile/mobile.go` and `internal/daemon/state.go` store private node key/join identity under the app-private state directory. Never publish those files or logs containing them.
- The control plane handles device names, public keys, assigned and underlay IPs/endpoints, node credentials, approval and network topology. OIDC paths can handle email, subject and display name. Review `internal/control/store.go`, `oidc.go` and actual operator configuration.
- WireGuard encrypts payloads between endpoints, including when relayed. The operator/relay can still observe traffic and connection metadata. Do not claim that encryption hides all metadata or protects a compromised endpoint.
- Hosted sessions transmit screen frames and accept remote gestures after the applicable permissions/consent. Viewer endpoints can see shared content. Session chat/files and optional recording/storage differ by platform; verify enabled Android code paths before making any universal retention claim.
- The AccessibilityService is not a disability-support tool. It ignores accessibility events/window contents and dispatches gestures. Screen frames come through separate MediaProjection consent, not accessibility text extraction. These distinctions must remain clear in listing, disclosure, video and declarations.

The detailed [Android privacy evidence](android-privacy-evidence.md) separates current platform data paths, new backup exclusions, operator retention/deletion gaps and unverified features.

## Declaration preparation

Before Create app, the console requires a Developer Program Policies confirmation and an export-law certification, including software with encryption functions. Do not treat draft creation as legally empty preparation. Record a supported factual basis first.

For accessibility, use the non-accessibility-tool route, describe actual remote-control gestures, separate disclosure and decline effects. Do not set the disability-tool flag to avoid review. For VpnService, describe the mesh VPN/network tool purpose and actual encryption path. A demonstration video must show the real app flow and complete disclosure, not a simulation presented as device evidence.

Export research established that the2021 rule removed notification for standard-crypto public-source/beta software, with non-standard cryptography retaining notification. Public-source availability alone does not answer every restricted-binary question. The alternative740.9(c) beta exception has importer statements, transfer restrictions and post-test disposal conditions. No filing, tester obligation or final legal classification has been made.

## Submission order

1. Fix actual tunnel data test and migrate/validate target36 candidate. Keep capture/input and physical-device gaps explicit.
2. Finish final-binary data-flow/policy/export evidence, privacy text and declaration/video drafts. Escalate only an unresolved substantive certification, not routine preparation.
3. Settle permanent pricing and exact named tester audience from user evidence.
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
