# Mnet

Your devices, one private mesh.

Mnet is a mesh VPN. Every device gets a stable `100.64.x.x` address, traffic
moves WireGuard encrypted, direct peer to peer when NAT allows it and through
an encrypted relay when it does not. You run the control plane, so no third
party ever sees your topology, and the relay only ever sees ciphertext.

## Pieces

| Binary | What it is |
| --- | --- |
| `mnet-control` | The control plane: node registry, approvals, auth keys, network maps, admin dashboard and the packet relay in one binary. |
| `mnetd` | The node daemon. Owns the WireGuard device, finds endpoints, picks direct or relayed paths, serves Magic DNS. |
| `mnet` | The command line that drives `mnetd`. |

## Quick start

Run the control plane anywhere with a public address:

```bash
MNET_ADMIN_PASSWORD= ./mnet-control --listen :8080 --base-url https://control.example.com
# prints a generated admin password if you do not set one
```

Open `https://control.example.com/admin`, sign in, create a network
(for example `home`), then either make an auth key or leave joins to approval.

On each device, as root (Linux, macOS) or Administrator (Windows):

```bash
./mnetd &                      # the daemon
./mnet up --server https://control.example.com --network home --auth-key mnet_...
# or without a key: the CLI prints an approval link for the dashboard
./mnet status                  # peers, paths, handshakes
./mnet ping other-laptop       # ICMP through the mesh
```

Magic DNS listens on the node's own mesh address: `other-laptop.home.mnet`
resolves from any node that points DNS at its mesh IP.

## How it works

* **Keys and identity.** Each node generates its own WireGuard key pair at
  first run. The private key never leaves the device. Joining sends only the
  public key.
* **Addresses.** The control plane hands out addresses from 100.64.0.0/10,
  the CGNAT carrier range, so mesh addresses never clash with LANs.
* **Maps.** Nodes long poll `/api/map`. Any change (join, approval, endpoint
  update, removal) wakes every node in that network at once.
* **NAT traversal.** The daemon runs STUN from the WireGuard socket itself,
  so the mapping it discovers is the one WireGuard uses, and reports it plus
  LAN addresses to the control plane. Peers rotate through each other's
  candidate endpoints until a handshake lands; WireGuard roaming keeps any
  path that starts working.
* **Relay.** When no direct path completes a handshake, the peer's endpoint
  becomes `relay:<pubkey>` and packets travel a WebSocket on the control
  server. Packets stay WireGuard encrypted; the relay forwards blind.
* **Magic DNS.** A tiny resolver on each node answers `A` records for
  `<node>.<network>.mnet` from the live map and forwards the rest upstream.

## Build

Requires Go 1.26+.

```bash
go build ./cmd/mnet-control ./cmd/mnetd ./cmd/mnet
```

Cross compile the clients:

```bash
GOOS=windows GOARCH=amd64 go build -o mnetd-windows-amd64.exe ./cmd/mnetd
GOOS=darwin  GOARCH=arm64 go build -o mnetd-darwin-arm64  ./cmd/mnetd
GOOS=linux   GOARCH=amd64 go build -o mnetd-linux-amd64   ./cmd/mnetd
```

Windows needs `wintun.dll` (from wintun.net) next to `mnetd.exe`.
Linux and macOS need root or `CAP_NET_ADMIN` for the TUN device.

## Layout

```
cmd/
  mnet-control/   control plane
  mnetd/          node daemon
  mnet/           CLI
internal/
  control/        node API, admin API, dashboard, JSON store
  relay/          WebSocket relay hub and wire protocol
  controlclient/  daemon side of the control API
  bind/           WireGuard UDP + relay socket, STUN interception
  daemon/         engine: maps, path selection, endpoint discovery
  dns/            Magic DNS resolver
  ifacectl/       address and route setup per OS
  stun/           minimal RFC 5389 client
  netmap/         shared types
```

## Status

Version 0.3.0. The mesh works end to end: join, approve, direct and relayed
traffic, Magic DNS, Android app, exit nodes and subnet routes. On the list
next: ACLs, SSO/OIDC login, key expiry.

Mnet is provided by Fintex Australia Pty Ltd (ABN 60 688 406 108), New South
Wales, Australia. Contact: contact@fintex.au.

## Known issues

- The end-to-end mesh test (`go test ./internal/daemon -run TestMeshEndToEnd`) exercises two
  in-process WireGuard devices over a real control server, direct UDP, and the relay hub.
- The relay forwards frames as opaque datagrams and cannot read WireGuard payloads; privacy
  depends on WireGuard end-to-end encryption, not on relay trust.
- Windows clients need `wintun.dll` beside `mnetd.exe` (see below).

## Exit nodes and subnet routes

Any node can offer to carry traffic for the rest of the mesh:

    # on the machine that should be the exit node / subnet router
    mnet up --advertise-exit-node
    mnet up --advertise-routes 10.9.0.0/24,192.168.50.0/24

    # on a device that wants to use them
    mnet up --exit-node gateway

The serving node turns on kernel forwarding and masquerading automatically
(Linux; see docs/operations.md for macOS and Windows). The client selects
an exit node by peer name or mesh IP; its default route moves into the
tunnel (split /1 routes, so the underlay default stays as a fallback) while
host routes keep the tunnel endpoint and the control plane reachable.
Advertised subnets become WireGuard allowed IPs on every peer, so traffic
for 10.9.0.0/24 flows to the router node and out its LAN. Deselecting the
exit node or rejoining without the flags cleanly removes the routes again.

## Mobile

- **Android**: `android/` holds a complete app (VpnService, join form, live
  status, `au.fintex.mnet`). `android/build.sh` produces a signed
  `android/out/mnet.apk` without Gradle; the gomobile core is
  `mobile/mnet.aar`. Install with `adb install android/out/mnet.apk`.
- **iOS**: `ios/` holds the SwiftUI app and the NetworkExtension packet
  tunnel provider, plus `ios/build-go.sh`. The xcframework build and
  signing need macOS, Xcode and an Apple Developer account; details and
  the exact remaining steps are in `ios/README.md`.

Both apps share the Go node engine through the `mobile` package:
`Prepare` joins and returns the assigned mesh address, the OS builds the
tunnel interface, then `Start` (Android fd) or `StartFlow` (iOS
PacketFlow) brings the mesh up on it.

## Validation

- Full unit and end-to-end suite: `go test ./...`
- Public smoke test: `scripts/public-validation.sh` deploys mnet-control
  behind a public tunnel, joins two real daemons over the public URL and
  exchanges encrypted traffic both directions.
