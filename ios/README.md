# Mnet for iOS

Mesh VPN client for iPhone and iPad: the same Go node engine as every
other platform, wrapped in a NetworkExtension packet tunnel.

## What is here

- `MnetApp.swift`: SwiftUI app entry point.
- `ContentView.swift`: join form (control server, network, auth key,
  device name) and live VPN status.
- `PacketTunnelProvider.swift`: the `NEPacketTunnelProvider` extension.
  It joins the mesh first to learn the assigned address, configures the
  tunnel settings (100.64.0.0/10 mesh route, explicit 198.18.0.53/32
  tunnel-local userspace DNS route, `.mnet` match domain), then hands `NEPacketTunnelFlow` to the
  Go engine through the `MobilePacketFlow` protocol (`StartFlow`).
- `build-go.sh`: builds `Mnet.xcframework` with gomobile.

## Status

Source integration only. No signed Mnet iOS release, TestFlight delivery or physical-device VPN/session test is established. The shared Go tests do not replace NetworkExtension entitlement, signing, consent and on-device validation. The XcodeGen spec still has an older marketing version; align it with a selected release before distribution. See [project status](../docs/STATUS.md).

The app source now selects an exact Mnet tunnel provider, refuses duplicate matching profiles and keeps refresh/disconnect lookup read-only. Preference load/save errors no longer become an assumed missing configuration. Only explicit Connect may save/enable a profile. See [profile ownership](../docs/ios-vpn-profile-ownership.md) for source/Swift-check scope and limits. This is not physical-device profile or VPN acceptance.

Packet-flow cancellation and full provider run ownership now have source/host-test/SDK checks, with exact results collected in CI. See [packet cancellation](../docs/ios-packet-flow-cancellation.md) and [provider ownership](../docs/ios-tunnel-run-ownership.md). These do not establish a linked, signed or physically accepted extension.

## Build

On a Mac with Xcode installed:

```sh
# Use the x/mobile revision pinned in the project module, not an unreviewed latest.
go install golang.org/x/mobile/cmd/gomobile
go install golang.org/x/mobile/cmd/gobind
gomobile init
sh ios/build-go.sh
```

The repository includes `ios/project.yml` for XcodeGen. Its source paths are relative to ios/; the app embeds the MnetTunnel target, which links the generated static Go framework. The [unsigned simulator linked-build check](../docs/ios-unsigned-linked-build.md) validates packaging without signing, provisioning, account access or launch. It does not authorize later signing; validate that separately for a selected delivery. The intended targets are app `au.fintex.mnet` and packet-tunnel extension `au.fintex.mnet.tunnel`, embedding the generated framework. The extension needs the packet-tunnel NetworkExtension entitlement. State is kept in the extension container; shared app-group behavior is not established by this guide.

## Remaining Apple steps

- Build the xcframework with the macOS/Xcode toolchain and verify the generated project and current marketing/build versions.
- Verify the selected developer team's bundle IDs, entitlement/provisioning access and signing. Account or paid-plan changes require separate authorization; this guide does not authorize them.
- Produce and validate a signed test build before any upload. No successful TestFlight result is claimed here.
- Test real device VPN creation, join, DNS, session viewing, reconnect, backgrounding and cleanup. Hosting through ReplayKit is not implemented.

Shared Go `PacketFlow` tests exercise the bridge in isolation, not Apple's production NetworkExtension lifecycle.
