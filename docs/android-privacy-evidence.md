# Android privacy and declaration evidence

Updated 2026-11-17. Submission preparation only. This is not a published privacy policy or a completed Google Play declaration. Review the final binary and actual operator setup before using any statement in a form.

## Android data paths

| Data | Current source behavior | Retention/deletion boundary |
| --- | --- | --- |
| Enrollment configuration | MainActivity stores server, network, enrollment key and device name in private SharedPreferences. MnetVpnService reads it when connecting. | Disconnect does not erase configuration. No in-app account/data-deletion flow is implemented. |
| Device identity | mobile/mobile.go and internal/daemon/state.go save node keys and join credentials under the app's private files/mnet directory. | Stop/Close release the running node, not its saved identity. Do not describe disconnect as deletion. |
| Network/control metadata | The control service processes device names, public keys, assigned/underlay addresses, endpoints, approval and network membership. OIDC code can process identity-provider subject, email and display name. | Actual operator storage, backups and retention must be specified. Admin node removal is not deletion of every related log or account membership. |
| Screen frames | HostSessionService captures after Android MediaProjection consent. It holds the latest JPEG in memory and supplies frames to the session engine. ViewerActivity decodes received frames for display. | Stopping hosting clears the service's latest-frame reference and stops projection. This does not prove erasure of copies made by viewers, platform buffers or a separate desktop recording feature. |
| Remote gestures | MnetAccessibilityService dispatches supported pointer gestures only while hosting. It does not retrieve window contents or consume accessibility events. | Accessibility is an optional remote-control mechanism, not a disability-support tool. Host consent and the separate Android permission remain required. |
| Chat | ViewerActivity sends chat through the session. HostSessionService displays incoming chat through an app-scoped broadcast; the viewing UI keeps visible chat text in memory. | No universal no-retention promise: another endpoint can retain content, and other platforms have different features. |
| Files | Shared session code supports file transfer. Android host options currently do not select a dedicated receive directory; the shared receiver defaults to its process working directory. | Do not advertise dependable Android file saving or claim that files are always ephemeral. A controlled Android storage path/UI and runtime proof remain open. |
| Audit metadata | internal/control/hardening.go appends event time, event type, client IP, actor, detail and outcome to an operator-side audit file when configured. | No automatic rotation or retention limit is established by that code. Node deletion does not purge the audit file. |

## Backup controls in new source

The manifest now explicitly sets allowBackup=false. Legacy full-backup rules exclude every documented storage domain. Android12+ extraction rules separately exclude every documented domain for cloud backup and device-to-device transfer. These settings direct the platform to exclude app data rather than clone an enrollment key or device identity.

Evidence: three XML policy tests, a negative control that removes the cloud SharedPreferences exclusion, SDK34 and SDK36 resource/Java/dex builds, and inspection of the packaged manifest. These are source/build checks, not an executed cloud backup/restore or every manufacturer's transfer utility. They do not remove any earlier backup, modify a previously delivered APK, or establish that a phone was ever backed up. Cross-platform transfer behavior on Android16 QPR2 also needs explicit runtime review before making a universal transfer claim.

Source: https://developer.android.com/identity/data/autobackup

A [notice review draft](android-privacy-notice-draft.md) now assembles those facts without inventing a retention period or adopted policy. Its operator verification table remains a publication gate.

## Decisions and evidence still needed

- Identify the actual beta control-service operator, storage and log retention, backup handling and supported deletion contact/workflow. Do not invent a fixed retention period.
- Write and review the privacy notice against that setup and the final Android binary. The notice needs a stable destination before forms or a listing point to it.
- Complete the VPN and non-accessibility-tool declarations using the actual consent flow. Record a real-device demonstration rather than labeling a simulation as a working phone.
- Do not answer Data safety with a blanket "no data collected". Apply Google's definitions to each operator/endpoint flow. Internal-only testing's form exemption is not permission to omit a truthful privacy notice or sensitive-permission disclosures.
- Verify Android files and any recording features independently from desktop support before mentioning them in the store listing.

The release gates and source URLs are maintained in [Google Play readiness](google-play-readiness.md). The store text remains an [unapproved draft](PLAY-LISTING.md).
