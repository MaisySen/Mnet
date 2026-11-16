# Mnet architecture

Development design. [STATUS](STATUS.md) separates implementation, test evidence, published builds and open work.

## Control plane: `mnet-control`

One process stores networks, identities and policies in a JSON file. Changes use private temporary-file replacement and file sync, with rollback on persistence failure. This is not distributed durable storage or a proven large-fleet design.

- `/api/*`: enrollment, credential-authenticated rejoin, approval polling, map polling, endpoint reports and relay WebSocket.
- `/admin/api/*`: password-session administration of networks, nodes, keys, ACLs, metrics and audit. New source adds explicit membership grant/revoke, with a bounded JSON-only same-origin operator route; see [status](STATUS.md) for verification.
- `/admin`: embedded dashboard.
- OIDC: configured issuer and explicit subject/verified-email admission policy, signed-token checks and nonce binding. Verified issuer/subject approval provenance is stored privately, separately from the node display label; it never grants membership. Separate private network/issuer/subject memberships gate an internal own-device query; an operator grant target is not caller authentication. User login/ownership remains unfinished. Tests use a local signed provider; live-issuer certification is not claimed.

Maps contain approved, non-expired peers permitted by the current pair/route policy. Long polls carry changes; local clients cannot learn unseen server policy changes while disconnected. Newer source also enforces known map expiry timestamps locally.

## Node: `mnetd` and `mnet`

The daemon owns the WireGuard device, endpoint discovery, path state and map application. The CLI calls a capability-protected local API on a Unix socket or Windows loopback TCP. Private state holds both the WireGuard identity and the control-plane credential, so file permissions/DACLs are a security boundary.

The custom WireGuard bind supports ordinary UDP endpoints and `relay:<pubkey>`. STUN uses the same UDP socket. Discovery reports suitable IPv4 underlay addresses, excluding the node's own overlay address. Bounded periodic direct retries must continue even when a relay handshake is healthy. The direct-path Linux regression stops the control/relay process before transfers to prevent fallback from hiding a failed direct path.

The relay scopes connections and packet destinations by network, revalidates current pair authorization and keeps bounded queues. Each frame contains a 32-byte key plus an encrypted WireGuard datagram. Relay encryption does not hide node/traffic metadata from its operator.

Magic DNS serves map-derived `.mnet` A records on the node's mesh address and forwards other queries upstream. Host resolver configuration is still required. Addresses use `100.64.0.0/10`, which may conflict with another VPN or carrier network.

## Sessions

Desktop/mobile sessions use ordinary OS TCP between mesh IPs, normally port 5909. The [session protocol](session-layer.md) multiplexes screen, input, chat and files. Host authorization binds a claimed node name to the connection's mesh source IP and applies host acceptance/allowlist policy.

There is no second session encryption layer; confidentiality depends on WireGuard and endpoint trust. Session parsing, file reception, capture and input introduce their own attack surfaces and need independent review. The relay may carry encrypted session traffic.

`internal/proofnet` is a small attributed userspace adapter used only by diagnostic proofs. Its classic-SACK setting is not a change to desktop/mobile OS TCP. See [reliability](session-reliability.md).

## Trust and operational boundaries

- The control plane knows device public keys, credentials, addresses, endpoints, approval and topology. Its operator is trusted for mesh admission and peer-key maps.
- A relay cannot decrypt WireGuard payloads, but can observe metadata or disrupt traffic.
- Local administrators/SYSTEM and the private state owner remain trusted. A compromised endpoint can see its own plaintext.
- HTTPS termination must sit in front of the plain HTTP control listener for non-loopback deployment.
- Pair-level ACLs are not per-application-port rules. Known expiry is different from discovering an offline admin revocation.
- Managed Linux RFC1918 subnet routing/serving has bounded route/NAT ownership and separate-LAN kernel proof with operator-enabled forwarding. Non-Linux/IPv6/public subnet routes and exit internet routing remain unsupported. See [operations](operations.md#exit-nodes-and-subnet-routes).
