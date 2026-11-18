# iOS VPN profile ownership

The app's original profile helper selected the first returned manager, discarded preference-load errors, and created/saved an enabled configuration when no record was returned. Refresh and Disconnect called that same helper, so simply opening the app could start configuration installation. An error was indistinguishable from an empty result. This is a source finding, not a reproduced physical-device incident.

The replacement selects only the exact `au.fintex.mnet.tunnel` provider and refuses duplicate matching profiles rather than guessing or deleting one. Refresh and Disconnect load only. A missing profile stays disconnected; only an explicit Connect may construct/save/enable one. Load/save/reload failures stop the affected operation and leave an error instead of proceeding with a guessed manager. Connect reloads the saved configuration before attempting start. Status is read from the selected system connection rather than setting Connected just because start returned.

The returned preferences are app-scoped. This does not mean Mnet can enumerate or operate arbitrary other apps' VPN profiles. The narrower concern is provider/duplicate selection within configurations returned to this app. The readable [configuration-model discussion](https://kean.blog/post/vpn-configuration-manager) quotes this platform boundary. [Apple's NEVPNManager reference](https://developer.apple.com/documentation/NetworkExtension/NEVPNManager?language=objc) is the primary API entry point.

## Verification scope

Six local Python source-wiring checks pass. They check exact-policy use, read-only lookup/refresh, explicit-create selection, failure propagation, save/reload/start ordering and no optimistic connected assignment or automatic removal. These are not Swift execution.

The existing macOS general CI job now runs nine host Swift selection tests and typechecks the actual app SwiftUI/NetworkExtension sources against the iOS Simulator SDK. Those results must be collected for the committed revision; script syntax and Linux source checks alone do not certify Swift compilation. This action does not launch a simulator, load/save actual preferences, start a tunnel, use credentials, sign or provision an app, or touch an Apple account.

There is no new UI layout or rendered-pixel acceptance. Error text and actual runtime permission/profile behavior remain subject to native UI/device checks. The packet-tunnel extension, Go framework linkage, XcodeGen project generation, entitlement/provisioning, signed delivery, real VPN startup/recovery and Multies enrollment remain separate unfinished gates. Existing manual development setup and its credential-storage limits are not made consumer-ready by exact profile selection.
