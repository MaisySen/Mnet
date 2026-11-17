# Android store listing draft

Not approved for upload or ready to paste. Mnet is a development test build, with physical-platform, privacy-disclosure and store-policy review outstanding. Do not present planned routing/session capabilities as verified store features. See [STATUS](STATUS.md) and the [current Play readiness gates](google-play-readiness.md).

## Candidate text for review

App name: Mnet

Short description: A development mesh VPN for testing private connections between your devices.

Description draft:

Mnet is an experimental mesh VPN and remote-session project. Devices join an operator-managed network and use WireGuard-encrypted direct or relayed connections. Test builds include device enrollment, approval, mesh addresses and session features. Capabilities and validation differ by platform. Do not use it for sensitive traffic or important file transfers yet.

Mnet is provided by Fintex Australia Pty Ltd (ABN 60 688 406 108), New South Wales, Australia. Contact: contact@fintex.au.

## Required review before submission

- Check the current Play Console policy, VPN declaration, target SDK and testing requirements against live official sources. This file does not assert current eligibility. The source manifest currently targets SDK 34; do not assume that meets submission rules.
- Verify account/team authority, selected app record, release track and tester audience. No upload or promotion is authorized by this draft.
- Use the existing upload-signing identity securely. Evaluate Play App Signing choices in the actual account workflow; do not change enrollment based on old draft instructions.
- Review physical-device VPN/session behavior and all screenshots. Prior Test Lab captures include virtual-device evidence; do not call every image a physical-device capture.
- Verify feature claims against the selected binary. Exit/subnet routing and physical capture/input are not established as ready.
- Complete data-safety, privacy-policy and content-rating answers from an actual data-flow review, not the old blanket "no data collected" draft.

## Data-flow inventory to review

The control plane handles device names, public keys, assigned/underlay IPs, endpoint addresses, node credentials, approval and network topology; OIDC may add email/subject identity. Admin/audit logs contain operational events and network metadata. A hosted operator handles that data. WireGuard payload encryption does not mean metadata is absent.

Session endpoints handle screen frames, input, chat and files; optional recording and local file/audit storage need accurate disclosure. Remote sessions can carry user-created content. Determine collection/sharing/retention terminology under the current store definitions before answering forms. No finalized legal or policy conclusion is recorded here.

## Artifact checkpoint

The older public test artifact is Android0.8.2/code14 in v0.8.2-test.1. A recipient-restricted0.8.3-dev/code15 APK was separately delivered on2026-11-17 and blocked by Play Protect. Newer source permission/disclosure/native-alignment/inset fixes are not in that delivered APK. Do not retry it or treat local debug builds as replacement releases. AAB generation requires a freshly generated matching AAR; see [operations](operations.md#android). Store graphics and app metadata must be reviewed against the selected release, not historical screenshots alone.
