# Complete setup and pairing: implementation plan

Status: proposed local design, not enabled enrollment. Source checkpoint before
this plan: Mnet3cb981b; Multiesd7dafd9 is the last validated bridge checkpoint.
The stopped Multies lifecycle review and possible test edit are excluded.

## The intended first journey

1. Install Mnet and launch its home. A trusted operator-supplied configuration
   identifies Multies and the registrar. An absent configuration shows setup
   unavailable, not a text field whose web-supplied value becomes trusted.
2. Choose sign in. Use the system browser and existing Multies session/login.
   Select an eligible existing workspace. No broad browser cookie reaches Mnet.
3. Show a distinct network-joining decision: exact computer key, workspace and
   operator-selected network. Existing List approval cannot satisfy this step.
4. After fresh consent and reviewed device-key proof, the registrar commits one
   enrollment and a recoverable credential receipt. The daemon durably saves the
   exact result before it reports joined. Only then offer a separate Connect.
5. Show current reachable, authorized mesh devices. A Multies approval row is not
   a reachable Mnet node. Join only by the registrar's exact immutable binding,
   never email, display label or a coincident public-key string alone.
6. Open one of three tools. Networking, Remote Desktop and Shared Desk keep
   separate authority. Remote Desktop still needs host view approval and a
   separate pointer answer. Shared Desk needs an explicit paired-device/input
   grant and its own permissions; an offline arrangement is not a live pairing.
7. Cancel, decline, restart, response loss and revoke must be first-class outcomes.
   None may silently replace an existing device or reuse an ended input grant.

The first-product policy suggestion is own-device networking followed by attended
view/pointer. It is not a selected policy. Workspace-wide discovery, unattended
access and automatic keyboard sharing are not part of this proposal by default.

## Current source joins that are missing

| Boundary | Existing implementation | Required change |
| --- | --- | --- |
| Native identity | `internal/daemon/identity.go` durably prepares the local key without enrolling | A reviewed daemon-local proof operation; do not export private key to browser/UI |
| Browser handoff | `internal/multies/bridge.go` has PKCE/loopback and memory-only List credentials | Separate enrollment purpose, consent and recipient-bound assertion |
| Multies authority | `src/server/services/mnet.ts` issues List only, with live eligibility and expiring approval records | Purpose-specific enrollment decision and current revalidation, separately authorized branch patch |
| Operator relationship | No workspace-to-network registrar configuration | Exact trusted origins/issuer/audience, immutable mapping and explicit enrollment policy |
| Registration | `internal/control/store.go:Join` creates pending public joins or uses a legacy auth key | Dedicated durable transaction, not Join followed by ApproveNode |
| Person ownership | `membership.go` and `identity_devices.go` separate operator membership and approval provenance | Enrollment transaction must bind both current authorized principal and created node; neither is inferred from email |
| Recovery | Daemon saves node ID/token; legacy join has its own save-retry behavior | Registrar receipt/recovery protocol bound to the same attempt/device, tested across process crashes |
| Device selection | Chooser reads current local daemon evidence | Registrar binding and connected daemon map must agree before presenting a network peer as ready |
| Revocation | ExpireNode bumps maps; DeleteNode deletes node/provenance/token | Enrollment tombstone, active relay withdrawal and a measured stale-peer denial bound, not only removal from an account list |
| Tool pairing | Existing synthetic attended flows and offline Desk layout | Real distinct device/session consent and OS permission adapters, with physical cross-device acceptance |

`SetNetworkMember` explicitly revokes account-query access only. Do not market
that action as device or mesh revocation. Current `DeleteNode` has no enrollment
receipt tombstone. These are source facts, not a finding from the stopped review.

## Operator handoff, one bundle rather than repeated guesses

The operator supplies or explicitly selects:

- Trusted Multies origin and immutable identity issuer; exact registrar origin
  and purpose audience. No production hostname has been selected by this work.
- Existing Multies workspace ID to existing Mnet network ID mapping, plus a
  revision so a changed mapping invalidates an in-flight enrollment decision.
- Who may enroll: proposed minimum is an explicit issuer/subject allowlist.
  Ordinary workspace membership alone is not silently sufficient. A role-based
  policy or allowing a whole workspace is a separate policy choice.
- Which service authenticates to which endpoint, who provisions its credential,
  and which party operates key rotation/revocation. Secrets are provisioned out
  of band, never stored in this document or an application log.

These values block activation, not synthetic local implementation. Runtime must
also verify that the named existing workspace/network exists and that the
principal is currently eligible. Syntax validation proves neither fact.

`internal/enrollmentconfig` now models and validates this *inactive* handoff:
HTTPS origin-only endpoints, explicit issuer/audience/revision, nonempty exact
workspace/network/principal tuples, unique workspace entries and defensive
copies. No file/environment loader, route, network client, store adapter or
production caller exists. Same-network mappings for two explicitly configured
workspaces are not rejected merely by inference; their disclosure policy still
needs review. This is not an authorization verifier or product setup switch.

## Proposed transaction and recovery boundaries

The Multies enrollment decision must bind immutable person and current session,
workspace, mapped network, mapping revision, registrar origin/audience, native
attempt, device public key, expiry and the enrollment-only purpose. An assertion
requires an authenticated Multies-to-registrar channel or a reviewed signed
assertion mechanism. A caller-created JSON struct is not that assertion.

The registrar must revalidate current enrollment authority immediately before
commit. Any distributed race between that check and Multies revocation requires
an explicit protocol and measured bound; two HTTP reads cannot be called atomic.

Within one registrar durable transaction, validate the one-use attempt/proof,
reject conflicting existing identity, allocate node/address, store principal
provenance and the enrollment receipt, consume authorization and persist the
credential-recovery state. Publish maps only after durable success. Persistence
failure must leave neither an approved orphan nor consumed-only authorization.
Do not retrofit this by chaining existing public Join and ApproveNode.

A lost-response receipt cannot be recovered by public key/attempt metadata alone.
Proposed recovery re-authenticates the same native owner through a reviewed
proof and request secret, returns only the same bounded receipt, and never
creates another node. Exact credential wrapping/rotation and expiry are review
items, not implemented promises. After a local credential-save failure the UI
must say setup is incomplete, preserve the original key and avoid a fresh Join.

Revocation must invalidate registrar receipt/recovery and node authorization,
withdraw relay access, notify connected peers and remove allowed peer traffic
within the reviewed maximum. Offline peers need expiring authorization rather
than indefinite old maps. Network authorization must remain separate from an
active view/pointer/Desk grant. No production delay is promised until measured.

## Work split and exact permission gates

### Unblocked in Mnet's isolated branch

- This source-backed plan and inactive, pure operator mapping validator/tests.
- Additional isolated persistence/recovery transaction prototypes and tests,
  provided they have no HTTP/store production adapter or proof activation.
- Local setup state/error modeling that never claims enrollment succeeded and
  never promotes a List row into a peer or a tool permission.
- Existing installed cancellation/recovery checks and later synthetic fixtures.

### Requires a new specific user answer

The actual user approved Multies sign-in and approved-device access on a separate
branch, nothing live. Adding enrollment authority is a distinct effect. Main has
asked whether a separate, off-by-default network-joining approval flow may be
added to that branch, with no deployment and separate screen/mouse approval.
Until the original reply is available and covers this change, do not implement
that Multies extension. No decision is needed to keep writing isolated Mnet tests.

### Requires operator configuration, not broader user consent

The exact bundle above and a verified provisioning route. Do not ask the user to
supply cryptographic protocol design. No production connection or configuration
mutation is part of the currently authorized work.

### Requires independent review before any routed use

- X25519 possession candidate, transcript/domain separation, challenge lifecycle
  and protection of the daemon's existing WireGuard private key.
- Authenticated cross-service enrollment assertion and the check/commit race.
- Atomic receipt/node/credential durability and lost-response recovery.
- Tombstone/re-enrollment semantics and actual connected-peer/relay revocation.

The inactive `internal/keypossession` experiment remains inactive. Neither these
tests nor ordinary developer review constitutes independent security approval.

## Staged acceptance, with no shortcut between stages

1. Pure config tests reject omissions, URL confusion, duplicate workspace and
   principal bindings, issuer mismatch, unknown/zero lookups and caller mutation.
2. Reviewed assertion/proof and durable transaction tests cover altered tuple,
   stale mapping, expired/revoked consent, replay, concurrent winners, storage
   rollback, process restart, response loss and local save failure.
3. Real local Multies + registrar + installed native journey, using disposable
   accounts/keys only: approve, decline, cancel, restart/recover, connect, exact
   device reconciliation, revoke during setup and while connected. This stage
   requires the Multies permission gate and reviewed protocol, not just config.
4. Two independent computers and real transport/OS permissions. Test reconnect,
   network loss, connected-peer revocation, attended host decline/End and Desk
   input handoff/teardown without stuck buttons/keys. Cross-build is not OS support.
5. Only separately authorized release work may select live config, sign, deploy
   or publish. Passing local stages creates no deployment authority.

## Evidence retained, not promoted

Installed Mnet3cb981b passes the local default attended home/chooser/view/pointer
journey with independent target pixels, two native clicks/one scroll/zero keys,
joined owners and returned home. Installed both-size pointer/SIGTERM remains the
earlier e738709 checkpoint. Both current source view-only sizes pass, including
preapproval absence and first-screen handling. Silent capture returns through
the actual 15-second failure home. Prefixes were removed. These are synthetic
same-computer Linux X11 fixtures, not working consumer enrollment or physical
cross-machine pairing. Shared Desk's installed offline arrangement/recovery is
not live keyboard/mouse sharing. Full Multies repository typecheck has its prior
memory-limit failure; selected strict-entry checks do not replace it.
