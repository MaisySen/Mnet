# Enrollment handoff to registrar: concrete next boundary

Status: proposed implementation contract for review, not runnable authority.
Source inputs are Mnet908d3d390cf251b10267414f423ec19e640bf38c and
Multies1b1c516da7bd383e4f11cd187b4acfa79d84a0e5. The actual current consent
preview works, but deliberately has no ticket, proof caller or registrar. The
stopped List lifecycle investigation is excluded from this design.

## Selected implementation direction versus unselected policy

Engineering direction proposed here: separate enrollment-only PKCE exchange;
sticky reservation per immutable native attempt; precommitted native-generated
node credential, hashed in the registrar; one atomic Mnet enrollment receipt/node
commit; authenticated idempotent status after response loss. This avoids needing
to retrieve an old secret from its hash or silently rotate a credential merely
because a response was lost. It is a proposed refinement of the earlier
rotation-on-recovery discussion, requiring focused review before routed use.

Not selected here: real origins/issuer/audience/service credentials or workspace
mapping; whole-workspace/role enrollment; network lease duration; account-event
networking effects; public-key/receipt retention; how many devices may enroll;
release/deploy/activation. Local fixture limits do not become product policy.
No screen/input permission follows from network registration.

## Existing slice is deliberately not upgraded in place

Current `mnet_enrollment_decisions` is session-cascaded, preview-only storage.
Every existing row remains unredeemable forever. No migration converts approved
preview rows into tickets. New native exchange must use new versioned routes,
new tables and an explicit independent feature gate, off by default. If a
version/purpose is absent, refuse rather than accepting the older preview or
List exchange. The current command remains a preview until a separately named
completed flow passes acceptance.

`src/server/services/mnet.ts` currently provides only helpers for existing
person/session eligibility. Reusing those checks is appropriate; reusing its
List code/token/credential/table or adding enrollment to its scope is not.

## Native durable intent, before browser/network request

New daemon-owned pending intent must be privately persisted before browser
opening. It contains:

- Version and enrollment-only purpose, random attempt ID and correlation state.
- Existing durable WireGuard public identity; no rotation or private-key export.
- Exact trusted Multies/registrar origin, issuer/audience and mapping revision.
- PKCE verifier and a random256bit native node-control credential, both secret.
- The SHA256 digest of that credential, for the protocol binding.
- Explicit phase and creation/expiry deadlines. No approved/enrolled flag yet.

The daemon generates the credential with a CSPRNG and saves it alongside its
existing private identity before any remote side effect. Never a user-selected
password, URL parameter, browser field or printed diagnostic. A failed save
creates no browser/network request. Existing enrollment cannot be overwritten;
conflicting pending attempts refuse or require explicit local cancellation.

This is not a second independent state store. It is a versioned field in the
existing daemon private state, written through the same lifecycle owner. Its
scope is one pending intent, with explicit restart validation and fail-closed
migration. No node-control client becomes usable until the committed receipt
is checked and the complete resulting state is durably saved.

## Exact browser authorization tuple

Launch carries only validated public metadata and correlation material:
protocol version/purpose, attempt ID, device public key, credential digest,
S256 challenge, literal loopback callback, state and platform. Full public key
is canonical32byte base64url, not the List thumbprint. The credential and PKCE
verifier never reach the browser. Query strings contain no node secret/ticket.

The browser loads current eligible mapped workspaces and shows the exact key,
network, purpose and incomplete/enrollment status appropriate to the real flow.
Prepare and decision compare all displayed operator tuple fields against server
config, as the current corrected preview does. Browser inputs are comparison
values, never issuer/network configuration. At decision, one conditional SQL
transition captures current person/session/workspace eligibility and stores the
complete immutable consent tuple. Displayed tuple mismatch refuses.

After approval the browser receives only a short-lived one-use authorization
code through the loopback redirect. Native checks exact state, callback owner,
origin and attempt, then exchanges code+verifier directly with Multies. Wrong,
repeated, expired, canceled or scope-mismatched exchange refuses without
consuming the intended owner's still-valid code. An enrollment-specific native
result is not a node credential and is never accepted by List/view/input APIs.

## Durable Multies journal, not a browser session receipt

New `mnet_enrollment_handoffs` table must not cascade away when a session/user/
workspace is deleted. Store immutable principal/session/workspace IDs as
provenance strings plus one-way secret hashes. Current eligibility is checked
against live auth tables by exact IDs at prepare/exchange/reserve; disappearance
refuses. Durable ticket/commit history is not an ongoing session entitlement.
Privacy retention and opaque tombstone cleanup must be designed explicitly.

Proposed fields:

- Opaque record ID; unique attempt ID; version/purpose.
- Principal/session/workspace; immutable complete operator tuple/revision.
- Device public key, credential digest, PKCE challenge, callback/state/platform.
- Status; authorization-code hash; enrollment-ticket hash; immutable reserve ID.
- Consent expiry, ticket expiry, reservation commit deadline, timestamps.
- Bound registrar service identity and eventual Mnet commit receipt digest/ID.

No plaintext credential/verifier/private key. Code/ticket plaintext is emitted
only at its own one-use delivery point; dropped responses do not cause fresh
secret delivery from a stored hash. Lost code/ticket before reservation requires
a new local intent and fresh consent, or an explicitly reviewed retry protocol.
Do not mutate a prior ticket back to issued.

State transitions, enforced by conditional updates with exact bindings:

| From | Trigger | To | Required effects |
| --- | --- | --- | --- |
| absent | valid authenticated prepare | pending | Unique attempt, live eligibility, captured tuple |
| pending | matching fresh person decision | approved or declined | Code hash only on approval |
| approved | exact code+PKCE exchange | issued | Code consumed, ticket hash stored once |
| issued | exact authenticated registrar reserve | reserved | Ticket tied permanently to one reserve/commit descriptor |
| reserved | identical reserve retry | reserved | Same public tuple and status, never a new ticket |
| reserved | matching committed receipt acknowledgement | consumed | Store receipt ID/digest, no return to issued |
| pending/approved/issued | expiry/withdrawal/current authority loss | terminal | No positive reuse |
| reserved | timeout or ambiguous status | uncertain | No new enrollment; status-only reconciliation |
| consumed/uncertain | authenticated status reconciliation | same terminal identity | Never reissue consent or create another reserve |

If an unused issued ticket expires, its secret cannot enroll later. If an
already reserved ticket becomes uncertain, retry cannot obtain a new reservation
for another descriptor. Expiry is not proof the registrar failed to commit.

## Service authentication and API shapes

All proposed routes are versioned enrollment-only endpoints, separately gated.
Exact path names below are proposed API names, not current capabilities.

Browser/native Multies surface:

- `GET /api/mnet-enrollment-v1/authorize`: public launch shape, current browser
  session required; no bootstrap of a workspace or membership.
- `POST /api/mnet-enrollment-v1/authorizations`: displayed tuple guard and fresh
  session-bound pending decision.
- `POST /api/mnet-enrollment-v1/authorizations/:id/decision`: one winner.
- `POST /api/mnet-enrollment-v1/exchange`: exact code+PKCE+public tuple, cookie/
  Origin rejected; enrollment ticket only, never node authority.

Registrar-to-Multies surface:

- `POST /api/mnet-enrollment-v1/service/reserve`: enrollment ticket, attempt,
  exact immutable commit descriptor and fresh reserve ID. Authenticate exact
  registrar service before ticket lookup. Live eligibility and tuple equality
  participate in the same conditional reservation transition.
- `POST /api/mnet-enrollment-v1/service/status`: same service and reserve/attempt,
  returns bounded state/tuple, not ticket plaintext or a node credential.
- `POST /api/mnet-enrollment-v1/service/ack`: same reserve and durable Mnet
  receipt ID/digest, idempotent same-result acknowledgement only.

Use a separately provisioned service credential for this exact origin/purpose,
never a browser cookie, List token, public key or operator-admin password.
Store configured credential digests, compare fixed-size digests in constant time
and support explicit key IDs for rotation. The native ticket is not service
authentication. Error bodies do not distinguish another person's ticket. No
redirects, arbitrary callback URLs, cookies or inherited ambient credentials.
A fixture uses disposable explicit credentials, never a production fallback.

A timeout does not make the native retry a new user decision. Every request has
an immutable idempotency descriptor. Rate/capacity limits must distinguish cheap
auth failure from actual per-principal reservation without disclosing existence.
Numerical values remain explicit local fixture settings until product policy.

## Registrar atomic transaction in the existing Mnet Store

A new routed registrar would still remain disabled until review and acceptance.
Its future transaction belongs in `internal/control.Store`, not a parallel store
or `Join` followed by `ApproveNode`.

Commit descriptor must include version/purpose, attempt, Multies handoff/reserve
ID, mapping revision, principal/session/workspace, exact issuer/registrar/audience,
network, device key and native credential digest. Proof binding must cover all
these fields or a collision-resistant canonical digest with a separately pinned
encoding. Existing inactive proof v2 lacks dedicated attempt/credential-digest/
reserve fields; it must not be called sufficient for this new descriptor. Any
v3 change needs vectors and independent review before a daemon adapter exists.

Immediately before commit, registrar obtains current authenticated reservation
state, checks its deadline/current operator mapping and validates a fresh exact
key-possession proof. No caller-created boolean claims proof or consent success.

Under one Store lock and one durable save, atomically:

1. Refuse conflicting attempt/reserve/device identity, expired/revoked/tombstoned
   authority or changed mapping. Never silently replace a legacy-present node.
2. Allocate node/address, store verified private provenance, native credential
   digest and registrar-origin enrollment metadata. No auth key is created.
3. Store immutable attempt/reserve receipt containing node/public tuple and
   consumed proof/challenge metadata; no duplicate receipt/node allocation.
4. Publish success/map notification only after persistence succeeds. Existing
   legacy token map stays for legacy nodes; registrar nodes use an explicit new
   digest-only credential variant, never a string guessed by token prefixes.

Repeated exact commit must return the same nonsecret receipt, not a new node or
secret. A changed credential digest or public tuple for the same attempt refuses.
The raw native credential is known durably to the native daemon before commit,
so response loss needs no server-held recoverable secret. After validating the
receipt, native saves node/network/server + credential as its active state and
only then may it construct a usable client. Connect remains a separate action.

## Status and recovery without secret redelivery

After timeout/restart, daemon uses its saved attempt/key/credential and exact
configured registrar. Receipt-status access is authenticated by its pre-saved
credential (verified against the committed digest) and an operation-specific
fresh proof if the reviewed protocol requires it. Public attempt/key alone
returns no private result. No new node, token rotation or new enrollment occurs
for a lost commit response.

If no commit exists, the saved intent may retry only the same exact descriptor
and sticky still-authorized reservation. Never use an absent response as evidence
of absent commit. An uncertain/expired reservation cannot automatically become
new authority. If the registrar cannot establish a safe retry, show setup
incomplete and require a new explicit consent, without replacing the old key or
silently deleting a possibly committed node.

If local state loses the saved node credential but retains the key, ordinary
lost-response recovery cannot succeed. A separate fresh-consent authenticated
rotation/reset is a different operation with tombstone/revocation checks and
requires its own design; do not add a key-only credential-retrieval shortcut.
If both key and state are lost, it is a new device, not identity recovery.

## Crash/failure matrix

| Fault point | Required persistent outcome |
| --- | --- |
| Native intent save fails | No browser/open/enrollment request |
| Browser canceled / wrong callback | No ticket exchange; intent incomplete |
| Code exchange response lost | No hash-to-secret recreation; fresh consent or reviewed exact retry |
| Service reserve response lost | Same descriptor retry/status only; never new reservation |
| Registrar proof fails | No node or consumed-only commit |
| Registrar save/fsync/rename fails before publish | No live node/receipt mutation; sticky remote reservation not reset |
| Registrar commits, HTTP response lost | Native pre-saved credential recovers same nonsecret receipt |
| Multies ack lost | Registrar receipt stays committed; authenticated idempotent status/ack, no ticket reuse |
| Native final save fails | No usable client; retry receipt with same intent/credential |
| Native process exits before final save | Pending intent survives; same receipt/status path |
| Device revoked before retry/status | Deny data/recovery, preserve tombstone, do not re-enroll |
| Mapping changes while pending | Refuse changed tuple, never silently remap |

Existing temp-file fsync+rename alone does not prove power-loss durability of the
containing directory or multiprocess exclusion. Implementation must establish
single-writer operation, directory-sync semantics and post-rename uncertain
errors before claiming atomic crash durability. Do not roll memory back to an
older version after a disk rename may already have published the newer state.

## What remains unresolved before activation

- Distributed current-authority check and local commit cannot be one atomic
  transaction. A deadline/lease and explicit revocation path are required, with
  measured residual window. Adjacent calls are not a subsecond guarantee.
- Valid reusable legacy auth keys are standing approval authority after
  delete/expiry, as current real-store matrix confirms. Registrar tombstones and
  route isolation must choose how that independent authority is constrained;
  no existing legacy behavior is silently changed by this plan.
- Node API, relay/queued packet checks and direct peers must converge on the
  same selected durable revoke/suspend semantics. Unknown offline changes need
  explicit expiring authorization, not optimistic indefinite maps.
- Public key/attempt/provenance retention and account-event effects need explicit
  product/operator choices. Do not hardcode logout as permanent device deletion.
- Physical two-computer networking and OS permission acceptance, resource/failure
  injection and mixed-platform work remain. CLI cross-build is not native support.

## Review and implementation sequence

1. Review this descriptor/state/recovery refinement, especially native-generated
   pre-saved credentials, no secret redelivery and durable uncertainty behavior.
2. Implement durable Multies journal and browser/native PKCE exchange, still no
   registrar consume/activation, with exact current auth and adversarial tests.
3. Review service authentication and sticky reserve/status/ack handlers, then
   integrate against real local Multies using disposable keys/config.
4. Implement reviewed atomic Store/daemon state changes with crash/failure tests,
   then reviewed proof adapter/descriptor. No legacy Join/Approve fallback.
5. Exact installed local end-to-end acceptance first, then physical two-computer
   revocation/OS/input acceptance. Separate release/config authority remains.

Neither this design nor source-only feedback activates a protocol. The current
working product evidence is still consent preview only, with the registrar
explicitly not connected. No enrollment or consumer readiness claim is added.
