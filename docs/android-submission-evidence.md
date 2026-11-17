# Android submission evidence pack

Updated 2026-11-17. Preparation only. This is not a completed Play declaration, legal certification, production launch or security approval.

## Candidate and distribution boundary

- Package: au.fintex.mnet. Source targets Android36, minimum24.
- Fresh current-source native bindings pass16KiB ELF LOAD checks on all4ABIs. The release-upload-key-signed AAB builds and validates with bundletool. Existing upload certificate matches the earlier delivered APK; no new signing identity or Play App Signing selection has been made.
- Current version remains0.8.3-dev/code15. Select a fresh final candidate/version and inspect actual Play signing enrollment before upload. Never substitute a debug AAB for the verified upload-signed candidate.
- No Mnet Play app record, binary upload, tester release or usable opt-in link exists. Owner selected Free to download on2026-11-17. Exact restricted tester audience is still being resolved outside this document. This choice is not permission to add billing, subscriptions, or a public launch. Do not add tester addresses here.

## Actual runtime evidence

[3b356e9 API34/36 run](https://github.com/MaisySen/Mnet/actions/runs/44033244020) passes:

1. Real APK/JNI startup and actual OS VPN consent.
2. Bidirectional4MiB hash, manual disconnect/reconnect and repeat hash. VPN-app sockets explicitly select the assigned mesh source.
3. A separately installed app UID sends ordinary unbound bidirectional TCP transfers twice.
4. Actual Mnet accessibility disclosure, Agree, Android Settings enablement, separate MediaProjection consent, and per-viewer Allow.
5. Real Go peer fully decodes screen JPEGs, sends a remote mesh gesture that clicks a disposable Android button through the actual AccessibilityService, and observes transport closure after hosting stops.

These are disposable emulator tests, not physical-phone, OEM battery-policy, long-soak, radio handover, reboot/unlock or Play Protect approval. A later API36 repeat timed out at frame readiness while API34 passed, so one success is not a universal reliability guarantee. The fixture now starts its viewer timeout only after projection approval to avoid counting Settings/recording preparation against the viewer window. A later full matrix atbee0906 passes both APIs again, but88d73f8 also had an intermittent API36 TCP-connect timeout. Continue final-candidate repeats and distinguish frame-readiness timing from TCP startup.

## VPN declaration draft basis

Purpose: private mesh networking and remote access between approved devices. VpnService establishes an IPv4 mesh route, not an unrestricted internet exit VPN. Source rejects the unsupported exit-node setting.

Transport: WireGuard encrypts payloads between node endpoints. Relay/control infrastructure can observe metadata. Do not say all data is invisible to the operator or that endpoints cannot retain received content.

Candidate prose for review, not submitted: "Mnet uses VpnService to connect approved devices on a private mesh network. It carries encrypted device-to-device traffic, including user-approved remote screen sessions, over WireGuard. It does not modify other apps' traffic for advertising or monetization."

Validate that last sentence against the final dependency/binary inventory, operator setup and current policy form before use. No advertising SDK is intentionally included by the present build, but source intent is not an automated dependency audit.

## Accessibility declaration draft basis

Mnet is not an accessibility tool for people with disabilities. Its optional service dispatches supported remote pointer gestures while the phone hosts a session. It does not retrieve window text/contents, subscribe to accessibility events or type text. Screen capture uses separate Android MediaProjection permission. Declining accessibility leaves mesh VPN, viewing and screen-sharing routes available, but disables remote gestures.

The declaration demonstration must show the real disclosure, decline/agree behavior, Settings enablement, screen capture, per-viewer approval, remote gesture and stop. A controlled emulator demonstration is evidence of that flow, not a physical device claim. The current recording stage is still being validated. Native Android screenrecord ended early across Settings/projection transitions even when the full integration passed. The new demonstration samples real screenshots through the controlled flow; it must be described as a sampled walkthrough, not uninterrupted video. The encoder must be present on the runner and the actual frames inspected before using it. Never include enrollment screens, keys, account data or private screens in a demonstration.

## Privacy/deletion review

[Android privacy evidence](android-privacy-evidence.md) owns source data paths and retention limits. A published notice still needs the actual operator's storage/log-retention/deletion handling and a stable notice destination. Device disconnect does not delete credentials. Admin node removal does not purge audit logs or account membership. Do not claim a deletion SLA or a blanket no-data policy without an implemented and owned process.

New source opts out of platform backup with explicit cloud/D2D and legacy exclusions; that is not proof every OEM honors it or that an earlier backup is removed. The earlier delivered APK is unchanged.

## Export-law review

Public-source standard-cryptography notification and restricted-binary distribution are different questions. The2021 BIS change removed notification for standard-crypto public source and beta software; non-standard cryptography retains notification. Public-source Mnet uses published WireGuard/X25519 and Go crypto, not a private algorithm. That is not by itself a final whole-binary export certification.

Three paths are being compared against actual distribution:

| Path | Grounded rule | Remaining factual check |
| --- | --- | --- |
| Public-source/corresponding object code | 734.17(b) and742.15(b);734.7(b) also refers to publicly available object code | Final binary/source correspondence and whether restricted Play delivery satisfies the applicable exclusion. Do not silently widen audience to force an exemption. |
| Owner's internal development | 740.17(a)(1)(i) permits certain private-sector internal development without classification/report submission | Actual exporter/end-user, eligible headquarters and internal-development use. Not a standing route for unrelated beta testers. |
| Restricted beta | 740.9(c) covers eligible beta software with stated conditions | Importer statement, testing destination/end-user, test-period/disposal obligations and intended public product. These obligations have not been imposed or accepted. |

No filing, importer undertaking or legal certification has been made. Do not treat a general "yes" to app work as factual proof of these conditions.

## Repeated engineering evidence

- [bee0906 actual API34/36 full matrix](https://github.com/MaisySen/Mnet/actions/runs/44038715928) and [general CI](https://github.com/MaisySen/Mnet/actions/runs/44038716115).
- Fresh uncached full repository `go test -race -count=1 ./...` passes at3fba224. This is local engineering evidence, not a phone or policy decision.

## Official and specialist sources

- https://support.google.com/googleplay/android-developer/answer/9845334?hl=en
- https://support.google.com/googleplay/android-developer/answer/10964491?hl=en
- https://support.google.com/googleplay/android-developer/answer/12564964
- https://developer.android.com/identity/data/autobackup
- https://www.ecfr.gov/current/title-15/subtitle-B/chapter-VII/subchapter-C/part-734/section-734.7
- https://www.ecfr.gov/current/title-15/subtitle-B/chapter-VII/subchapter-C/part-734/section-734.17
- https://www.ecfr.gov/current/title-15/subtitle-B/chapter-VII/subchapter-C/part-742/section-742.15
- https://www.ecfr.gov/current/title-15/subtitle-B/chapter-VII/subchapter-C/part-740/section-740.17
- https://www.ecfr.gov/current/title-15/subtitle-B/chapter-VII/subchapter-C/part-740/section-740.9
- https://www.bis.gov/media/documents/table-changes-enc-wa2019-rule-final-version.pdf
- https://bestpractices.linuxfoundation.org/regulatory/export/non-standard-cryptography.html

The current regulation and BIS2021 change table narrow older explanatory pages that still describe notification as universal. The source checklist, not an old broad summary, owns the submission decision.
