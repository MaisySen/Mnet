# Mnet architecture

Mnet has three moving parts and one rule: no third party ever sees plaintext.

## The rule

Traffic between nodes is WireGuard encrypted end to end. The control plane
sees keys, addresses and endpoints, never packet contents. The relay
forwards ciphertext it cannot read.

## Control plane (mnet-control)

One binary, one JSON state file, no database to run.

* **Node API** (`/api/*`): join (auth key or admin approval), map long
  polling, endpoint reporting, relay WebSocket.
* **Admin API** (`/admin/api/*`): networks, approvals, auth keys, node
  removal, behind a session cookie from the admin password.
* **Dashboard** (`/admin`): a single page app served from the binary.
* **Relay hub**: one WebSocket per node; frames are
  `[32 byte key][wireguard packet]`, destination key toward the server,
  source key toward the client.

A network map is the full list of approved peers with their keys,
addresses and endpoints. Nodes long poll; any change wakes them at once.

## Node (mnetd + mnet)

* `mnetd` runs the engine; `mnet` drives it over a local API (unix socket,
  TCP loopback on Windows).
* **bind** is the WireGuard socket: UDP plus relay in one conn.Bind. Peers
  carry a normal UDP endpoint or `relay:<pubkey>`. WireGuard roaming moves
  a peer to any path that starts working, direct or relayed.
* **Endpoint discovery** runs STUN from the WireGuard socket itself. The
  NAT mapping discovered is the mapping WireGuard uses. LAN addresses are
  reported too, for same network peers.
* **Path selection** rotates a peer's candidate endpoints while handshakes
  fail, falls back to the relay, and retries direct every 90 seconds.
  Persistent keepalives keep NAT pinholes open.
* **Magic DNS** answers `<node>.<network>.mnet` (and `<node>.mnet` when
  unambiguous) from the live map on the node's own mesh address, and
  forwards everything else upstream.

## Addressing

Nodes get addresses from 100.64.0.0/10 (CGNAT space), allocated by the
control plane, stable per device key pair.

## Trust boundaries

* Node identity is its WireGuard key pair, generated on device. The private
  key never leaves the device.
* A node token (issued at join) authenticates map, endpoint and relay
  calls. Approval gates everything a new node can do.
* The admin password protects the dashboard. Keep the control plane behind
  HTTPS in production (see docs/operations.md).
