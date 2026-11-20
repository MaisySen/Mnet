# Registration boundary, proposed contract

Status: design only. No registration route, membership mutation, deployment or ordinary-launcher change is implemented by this document. The current Multies bridge remains List only.

## What exists

- `internal/control/store.go: Store.Join` takes a network and public WireGuard key. A new public join creates a pending node and a bearer control credential. Operator approval or a network auth key separately approves it. Reusing an existing public key requires its saved node ID and control credential.
- `internal/control/membership.go: SetNetworkMember` is operator-managed. `IdentityDevices` requires the exact verified issuer/subject plus network membership. Its revocation does not revoke device credentials or mesh traffic.
- `internal/control/identity_devices.go` returns approved devices linked to verified approval provenance, not email/name/domain matches.
- `internal/control/server.go` exposes no Multies registrar route. The Multies client has no authority to call operator APIs.
- Multies workspace eligibility and its List credential are not Mnet network membership, registration authority or a node credential.

## Required independent authorities

1. An operator-configured exact mapping of eligible Multies workspace to existing Mnet network, trusted issuer and registrar audience. A client cannot choose a control origin, network or issuer by supplying JSON. Mapping changes require separate operator authority and audit.
2. A fresh person decision allowing enrollment of this exact device public key into that network. It must bind person/session, workspace, network, control audience, protocol version, device key and one-use request. List approval cannot be upgraded. Any future implementation needs its own purpose-specific permission and browser wording.
3. Proof that the requesting native process holds the corresponding WireGuard private key. PKCE proves possession of a fresh verifier, not ownership of the supplied WireGuard key. A browser-supplied thumbprint is not proof.
4. An atomic registration transaction that consumes the enrollment decision once and creates the network node, private approval provenance and control credential together. Public legacy Join followed by ApproveNode is not an atomic registrar.

## Key-possession candidate, not an enabled protocol

WireGuard uses X25519 keys, not Ed25519 signing keys. Do not reinterpret or convert a WireGuard private key as a signing seed. An enrollment-specific X25519 challenge can demonstrate agreement with a fresh server ephemeral key, with standard library `crypto/ecdh`, HKDF and HMAC. It needs security review before any routed use.

The server would retain a fresh challenge identifier, nonce, ephemeral private key, short expiry and a hash of the entire authorized request. The native client must obtain the challenge only through its exact trusted registrar connection, not a browser callback or arbitrary remote URL. The transcript must bind a protocol/domain label, version, exact registrar origin and audience, workspace, canonical network ID, verified subject, canonical device key, server ephemeral public key, nonce and authorization identifier. Use unambiguous length-prefixed or canonical encoding, not concatenated attacker-controlled strings. Derive an enrollment-specific MAC key, never expose the raw agreement result. Reject low-order keys and failed/all-zero agreement. Compare MACs in constant time.

A valid MAC proves key possession only. It neither verifies a person's authority nor creates network membership. Consume challenge and enrollment authority in the same durable transaction. Reject replay, stale mapping, logout, revoked person/workspace, changed key/network/origin, duplicate fields and concurrent second redemption. A response lost after a successful commit needs a bounded authenticated recovery design, not a second registration or a new credential returned using only public metadata.

A separate generated signing key with a challenge signature is another option, but only after a reviewed binding to the WireGuard key. Proving that unrelated signing key alone is insufficient.

## Lifecycle and revocation required before release

- Persist a node credential privately and only after the registration commit. Keep it out of browser URLs, UI diagnostics, audit bodies and logs. Define restart and interrupted-save recovery without silently enrolling a second node.
- Define workspace/person/session revocation effects on Mnet authorization independently of List eligibility. Removing account-read membership is insufficient. Stop refreshed maps and relays, revoke node control authorization and arrange peer authorization removal with an explicit maximum delay. Existing peers and existing tunnels must be considered, not only the next HTTP read.
- Device revoke must preserve identity/tombstone semantics so an old credential cannot rejoin and resurrect an approval. Separate device expiry, permission withdrawal and account unlink operations.
- Remote Desktop, Shared Desk, Transfer and Record need their own explicit purpose grants and current authorization. Network membership alone enables none of them.
- Default app setup needs a reviewed origin supplied by the product/operator configuration, not a guessed production hostname or a first arbitrary URL entered by a web page.

## Evidence gates

`internal/control/multies_separation_test.go` runs actual current handlers and documents current separation: a List-shaped bearer is not accepted as operator, account or existing-node authority; legacy new join stays pending, has no verified principal and cannot read a network map. There is no registrar alias. These tests are not enrollment implementation or proof of cross-service revocation.

Before enabling registration, require tests for valid key proof, wrong key, low-order server and client keys, nonce/transcript alteration, domain separation, request replay, concurrent redemption, membership and mapping races, approval logout, transaction rollback, interrupted credential save, device revoke/rejoin, relay withdrawal and stale connected peers. Then run an exact installed native journey against the real local registrar and Multies service. Existing List-only browser/native acceptance does not satisfy these gates.
