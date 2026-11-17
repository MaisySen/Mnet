# Mnet Android development-test privacy notice - review draft

Updated2026-11-17. **Not published or ready to use as a privacy-policy URL.** The operator verification section at the end must be completed before adoption. This text describes the current source candidate, not an older installed APK or a general public service.

## Provider and scope

Mnet is provided by Fintex Australia Pty Ltd (ABN60 688 406 108), New South Wales, Australia. Contact: contact@fintex.au. The owner confirmed on2026-11-17 that Fintex is the legal operator, that this address handles support and data-deletion requests, and that the inbox is monitored. This does not establish a response deadline or an automatic deletion process.

Mnet connects approved devices on a private mesh network and supports optional, user-approved screen sharing and remote gestures. The current development test is restricted to the owner's selected test account. A future Multies integration is planned, not part of this notice or a present data-sharing claim.

## Information processed

- On your phone, Mnet saves the control-server address, network, enrollment key and device name in app-private settings. It saves the device's private node identity and join credentials in app-private files. Disconnecting does not remove those settings or credentials.
- The configured control service processes device names, public keys, assigned and connection IP addresses/endpoints, enrollment credentials, device approval and network membership. Operator audit events can contain event time, client IP, actor, event detail and outcome. If an operator enables identity-provider login, that flow can also process the provider's subject identifier, email and display name. The operator must confirm which features are enabled for the test.
- When you start screen sharing and accept Android's screen-capture permission, Mnet captures and sends screen images to a viewer you approve. An approved viewer can see sensitive information displayed during sharing. Stop sharing before opening sensitive screens.
- Optional remote-control accessibility access lets an approved connected viewer send supported taps and scrolling gestures during hosting. The accessibility service does not read window text or contents, receive accessibility events, or type text. Screen images use a separate Android screen-capture permission.
- Session chat and file-transfer code can process content sent between endpoints. Android file saving is not a verified supported feature in this candidate; do not depend on it for document storage. Other Mnet platforms can have different recording/storage features and are not covered by an Android-only retention statement.

## Purposes and recipients

Mnet uses enrollment and connection data to identify approved devices, build the private mesh, maintain connections and enforce access decisions. The control service uses operational and audit data to operate and secure the service. Screen images, gestures and chat are sent to the session endpoint selected or approved by the user. A recipient can make copies; Mnet cannot guarantee deletion of another endpoint's copies.

WireGuard encrypts mesh payloads between endpoints. A configured relay or service host can still process connection metadata. Encryption does not hide the content from an authorized viewer or protect a compromised endpoint. Before publication, identify the actual control/relay hosting providers, locations and any additional recipients in the operator verification below.

## Permission choices

You can decline the accessibility disclosure. It leaves you in Mnet without enabling the service; remote gestures then remain unavailable. You can later reopen the disclosure and choose whether to enable Mnet in Android Settings. VPN and viewing do not require accessibility permission. Screen sharing separately requires Android's screen-capture permission and, unless explicitly enabled for a hosting run, a viewer-approval decision.

You can stop hosting and disable Mnet in Android Accessibility Settings. Disconnecting the VPN stops that connection but is not a deletion request. Platform app-storage controls can remove local app data; separately address server-side records and any copies held by other endpoints.

## Storage, retention and deletion

The app's latest encoded host screen frame and displayed viewer/chat state are held in memory by the current Android paths. Stopping hosting drops the service's latest-frame reference and ends projection. This is not guaranteed erasure of platform buffers, received content, desktop recordings or another endpoint's copies.

Saved local enrollment/identity can remain until app data is removed. Current source disables platform app backup and excludes app domains from cloud/device transfer rules, but OEM behavior and earlier backups are not covered by a universal no-backup promise.

The current control service has no automatic audit-retention limit. Removing a node does not remove every audit event or account-membership record. Do not treat a disconnect, logout, node-removal action or test-service restart as a complete deletion workflow. You can contact contact@fintex.au for support and data-deletion requests. The inbox is monitored, as confirmed by the owner. The operator must establish and document the actual supported handling, retention and backup process before adopting this notice. No response-time or deletion-deadline promise is made by this review draft.

## Operator verification required before adoption

| Item | Required evidence or decision |
| --- | --- |
| Live service | Exact control/relay operator, hosting providers, enabled identity features and deployment locations. Keep credentials and private URLs out of public records. |
| Retention | Actual live audit/proxy/provider logging, persistent storage and backups; choose and implement a defensible retention process rather than inserting an invented duration. |
| Deletion | Verify the support mailbox can receive requests; document who handles them and how local, control, account-membership, audit and backup records are treated. |
| Final binary | Confirm data paths, permissions, dependencies, exported components and lack/presence of analytics or advertising against the actual signed candidate. |
| Destination | Review wording and publish a stable, accessible notice for the exact candidate/service. An internal source file is not a usable policy URL. |
| Play forms | Apply Google's current definitions to the actual flows. Internal-only Data safety exemption does not eliminate privacy/sensitive-permission obligations. |

Source implementation inventory and official policy references: [privacy evidence](android-privacy-evidence.md), [submission evidence](android-submission-evidence.md), [Play readiness](google-play-readiness.md).
