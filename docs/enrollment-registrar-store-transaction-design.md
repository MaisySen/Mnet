# Registrar transaction in the owning Store, proposed connected contract

Preparation after eb932cfe lifecycle review. No registrar production types,
transaction method, credential transport, proof adapter or consumer added here.
This document deliberately does not invent a callable proof=true API.

## Actual integration gaps

Current Multies reserve descriptor encoding has exactly 19 fields including
platform; preserve its 570-byte vector. The reserve response has handoffId,
reserveId, descriptorDigest, deadline, reserved status and enrolled:false. It
does NOT include verified subject/person-session provenance. Current status is
historical, not a fresh live-eligibility authorization: active pinned service can
read it after person/session/membership changes. Thus status=reserved before the
deadline cannot supply a current authorization check. An exact reserve retry does
recheck eligibility, but remains a separate-system operation before Store commit.

The existing inactive keypossession v2 transcript binds subject/session but NOT
native attempt, credential digest or immutable reserve descriptor/deadline. Its
in-memory Pending consumes proof outside Store persistence. It cannot be bolted
onto this transaction or treated as durably consumed proof. No response supplies
a validated subject/session for it. No new protocol version is implemented here.

A concrete first internal transaction can be tested with explicit local fixture
inputs and no HTTP consumer. A routed adapter must wait for an authenticated,
reviewed source of the full commit binding and a revised proof transcript binding
attempt/reserve/credential/descriptor plus existing independent expected tuple.
No inference from email, browser JSON or response-selected registrar is allowed.
Do not quietly remove subject/session to make old proof v2 fit a new protocol.

## One persisted versioned namespace

Propose optional `registrar_state` member of existing storeData. Omission alone
means legacy file with no registrar history. Presence requires exact version 1
and nonnull records map. Explicit null, wrong/unknown version, duplicate members,
unknown fields or malformed records refuse Open without rewriting state. Once
present, it is never silently reset. New namespace is validated semantically on
load in addition to existing legacy shape validation. State file schema is not a
transport and secrets never appear in it.

Records map key: canonical JSON tuple [networkId, devicePublicKey], with key
canonicalized by decoding exact 32 bytes then standard-base64 encoding as used
by actual netmap.Node. Multies base64url key is converted by bytes, not accepted
as a second Store identity. Every record retains this immutable network/key
claim after node deletion. This is the guard preventing old legacy Join from
creating a replacement registrar-controlled identity under a reusable auth key.
Other networks and genuinely unrelated legacy identities keep their old policy.

Record fields (proposed exact storage contract for failing-first tests):
`network_id`, `device_public_key`, `node_id`, `credential_digest`, `phase`,
`scope`, `descriptor_digest`, `descriptor_encoding_version`, `receipt_id`,
`receipt_digest`, `committed_at_ms`, `access_expires_at_ms`.
Scope fields: `issuer`, `registrar_origin`, `audience`, `service_id`,
`workspace_id`, `mapping_revision`, `handoff_id`, `attempt_id`, `reserve_id`.
Phase is active or revoked. Deleted/expired/revoked records remain qualified
history, not reusable grants. This initial schema does not store subject/session
or imply account-device provenance. Any later provenance addition requires its
verified source and strict schema version decision before implementation.

Bounded capacity is a required isolated fixture input, checked BEFORE generating
IDs/allocating IP. No arbitrary eviction or production retention duration. A full
store refuses new registrar authority but preserves reads of existing receipts
and revocation. Disk growth/privacy retention remains a product decision.

## Atomic transaction, no Join+Approve chaining

Under the existing Store.mu and retained lease, check health first. Exact
independently configured tuple/current local revision, service identity, fresh
reserve authorization and proof validation must already be represented by a
non-exported immutable permit owned by the eventual verified adapter. Store must
not accept caller booleans, unvalidated response objects or a public constructor
that manufactures a permit from a matching digest. No such adapter exists now.

Check same-qualified historical receipt BEFORE considering creation. Exact retry
returns the same immutable receipt only and an explicit current-access state;
expired permit cannot allocate, revive, extend access or change a node. A receipt
is history, not a bearer credential or positive enrolled/current-authority flag.
All fields including service/attempt/reserve and both encoding versions must
match. Changed descriptor, credential digest, selected tuple or receipt scope
refuses with no save/version/IP/key-use change. Multiple same-attempt winners,
reserve reuse and conflicting key claims fail load or commit validation.

For NEW creation only, reject existing node with same network/raw key, even if
legacy-approved, and reject any standing registrar claim. No silent migration,
identity takeover or credential rotation. Reject expired/deadline-equal permits,
revoked/changed local tuple and insufficient event/record capacity. Explicit
fixture access expiry must be later than commit time and bounded by explicit
permit limits; never inherit legacy DefaultKeyExpiry as new grant policy.
Reservation expiry and access expiry are different values with different roles.

Prepare one candidate: node ID/name/address with no routes/exit capability,
credential digest variant, immutable qualified receipt and consumed permit/proof
identity in the same Store image. No entry in legacy NodeTokens and no AuthKey
consumption. Save once through owning persistence, then publish one map change.
Prepublication error restores all fields/maps/cursors and returns no receipt.
Postpublication uncertainty freezes ALL authority, returns no success receipt,
and cannot initiate another attempt. Owning reopen reads one whole image; exact
retry discovers the original qualified receipt or confirmed absent transaction,
never mints a new credential because a response was lost. Multies ack follows a
durable receipt and is historical only; its failure cannot undo the Store commit.

## Credential scope and receipt encoding

Native pre-saves random 256-bit secret, not user password. Store saves only its
SHA256 digest, exact canonical unpadded base64url 32-byte value. Digest is public
binding data, never accepted as the credential itself. Proposed future transport
uses a separate explicit scheme/endpoint, not prefix guessing inside legacy
X-Node-Token, query strings or URLs. It authenticates one node on one network and
one receipt epoch for node maps/endpoints/relay, not enrollment, membership,
admin, proof signing or screen/input. No credential endpoint is added now.
Legacy AuthorizeNode must refuse every registrar node regardless of a corrupt
mixed NodeTokens entry. New credential verifier must refuse closed/frozen,
revoked, expired or mismatched nodes before any map/relay delivery.

Qualified receipt identity is [issuer, registrarOrigin, audience, serviceId,
handoffId, attemptId, reserveId, descriptorDigest, receiptId]. Local receiptId is
CSPRNG 32 bytes unpadded base64url. Persist exact bytes/digest once, including node
ID/network/public-key/digest/access-expiry and committed-at, no private material.
Define its separate canonical encoding version before implementation. Canonical
reserve bytes must match JavaScript JSON.stringify, including no HTML escaping,
no Go trailing newline and exact UTF-8. Do not reuse Go encoding/json defaults
without cross-language vectors, especially U+2028/U+2029/HTML/non-ASCII values.
Current Multies ack only stores receipt identity/digest and does not validate a
Store commit; its receiptId scope is qualified, not globally unique.

## Legacy lifecycle isolation

Join checks the permanent registrar claim immediately after resolving the exact
network and raw key, before node/token/key-use/IP handling. Reject with no side
effects whether claim is active or revoked, node present or deleted, credential
correct or absent, key valid/reusable/consumed/expired/removed. No fresh pending
legacy node is allowed for claimed identity. Existing unclaimed legacy behavior
and reusable-key regression matrix remain unchanged.

ApproveNode/ApproveNodeAs/ApproveNodeIdentity reject registrar nodes instead of
reactivating expired authority or adding unverified principal provenance.
ExpireNode persistently marks the claim revoked as well as expiring the node;
DeleteNode atomically revokes the claim and removes node/credential but retains
receipt and raw-key guard. Neither deletes history. Failure rollback and uncertain
freeze follow existing owning lifecycle. No new revoke-on-logout/workspace-change
policy is implied; exact local operator revocation is separately effective.
No re-enrollment/recovery grant is implemented in this first contract. Reuse of a
revoked raw key needs an explicit future replacement-epoch policy, not deletion
of a tombstone by an ordinary operator endpoint.

## Substantive remaining tradeoffs before a consumer

1. Distributed check-to-commit: exact reserve retry is fresh but cannot atomically
   couple Multies eligibility to Store rename. Neither historical status nor a
   short local TTL removes the race. Need an explicit bounded authorization lease
   and revocation semantics acceptable for activation, or a stronger coordinator.
   Pauses/clock skew/storage latency must be tested at final publication, not only
   preflight. No arbitrary safe window is selected by this document.
2. Provenance/proof: current reserve response lacks required verified principal/
   session data and old proof v2 lacks reserve/credential binding. Requires
   reviewed protocol additions before a real permit issuer can exist. Preparation
   can test Store fixture semantics without pretending it authenticates anything.
3. Native secret auth: separate transport and daemon state are still absent.
   Avoid widening legacy token API merely to make a fixture look end-to-end.
4. Retention/access expiry/re-enrollment: explicit fixture values only, no default
   production lease/retention policy or implied account-event effects. Receipts
   retained after deletion contain linkable identifiers, so privacy cost remains.

## Failing-first acceptance checkpoint

Opt-in actual-Store tests use `registrar_transaction_contract`. They load/write
existing Store state with the proposed versioned namespace, not a fake registrar.
They assert malformed registrar-state rejection and permanent tombstone denial of
all legacy rejoin modes after actual Close/reopen, preserving disk bytes, IP and
key uses. Current Store ignores the namespace so these fail, demonstrating the
precise missing seam rather than reporting preparation as a working transaction.
Transaction method/proof issuance/credential endpoints are intentionally absent.
Future transaction acceptance must additionally exercise concurrent same/different
permit winners, capacity, mixed credential rejection, qualified receipt conflicts,
exact immutable retry, all persistence fault stages, revoked/expired late history,
and one real restart with the native presaved credential. No test can replace
external authentication or native OS acceptance.
