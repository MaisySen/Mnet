# Mnet for iOS

Mesh VPN client for iPhone and iPad: the same Go node engine as every
other platform, wrapped in a NetworkExtension packet tunnel.

## What is here

- `MnetApp.swift`: SwiftUI app entry point.
- `ContentView.swift`: join form (control server, network, auth key,
  device name) and live VPN status.
- `PacketTunnelProvider.swift`: the `NEPacketTunnelProvider` extension.
  It joins the mesh first to learn the assigned address, configures the
  tunnel settings (100.64.0.0/10 route, Magic DNS on the node's own mesh
  address, `.mnet` match domain), then hands `NEPacketTunnelFlow` to the
  Go engine through the `MobilePacketFlow` protocol (`StartFlow`).
- `build-go.sh`: builds `Mnet.xcframework` with gomobile.

## Build

On a Mac with Xcode installed:

```sh
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
gomobile init
sh ios/build-go.sh
```

Then create an Xcode project with:

- App target `au.fintex.mnet`, embedding `Mnet.xcframework`.
- App extension target `au.fintex.mnet.tunnel` (Network Extension,
  Packet Tunnel), embedding `Mnet.xcframework`.
- App group `group.au.fintex.mnet` on both targets (the extension keeps
  node state there).
- Entitlements: Packet Tunnel (`com.apple.developer.networking.networkextension`
  with `packet-tunnel-provider`) on the extension.

## Blocked on the Apple side

These cannot be done without Apple hardware and accounts, and are the
only remaining steps:

1. macOS + Xcode: `gomobile bind -target ios` needs the Xcode toolchain;
   the cross compile cannot run on Linux.
2. A paid Apple Developer account for the NetworkExtension entitlement
   and for signing/distribution (TestFlight or App Store).
3. On-device testing.

The Go engine itself is platform-neutral and already runs the iOS code
path (`StartFlow` over a `PacketFlow`) in the shared test suite.
