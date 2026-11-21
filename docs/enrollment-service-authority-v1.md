# Enrollment service-authority boundary, proposed next design

Status: design only, no implementation, routes, migration, service credentials,
node activation or policy approval. Baseline Multies9f622844 and Mnet898f2ec4.
The corrected journal/PKCE source review ran no tests. Builder validation remains
its separately recorded45affected tests and actual two-binding browser fixture.
Correction acceptance is not integrated enrollment or security certification.

## Authority and trust model

Multies is the consent/eligibility authority for its configured immutable local
subjects and workspace mappings. Registrar is a trusted network authority, not
an adversary constrained merely by values it can ignore in its own storage.
TLS authenticates configured peers; it does not make arbitrary callback or
response URLs trusted. Browser state is a comparison guard only. Native public
keys, attempt identifiers and credential digests are public binding material,
never credentials. PKCE proves the exchanger knows its verifier, not possession
of the device WireGuard private key or durable native secret storage.

No existing List token, browser session, enrollment ticket, preview decision,
public key or proposed node-control digest authenticates a registrar service.
Screen/input permissions remain independent. No new standing authority arises
from this design or its review.

## Native allowed-map versus selected-map, before an adapter

Resolve the apparent conflict between saving a mapping before browser launch
and learning the final workspace only after browser consent:

1. Before launch, native accepts an explicitly installed/operator-managed trust
   configuration containing one exact Multies origin/issuer and a finite set of
   allowed tuples: workspaceId, networkId, registrarOrigin, audience and mapping
   revision. No inference from email domain, browser-selected workspace, API
   response, DNS discovery or previous unrelated service is permitted.
2. Save a private versioned pending intent containing the exact trusted launch
   origin, a fingerprint of that canonical allowed-map snapshot and its revision,
   original launch tuple, PKCE verifier and proposed private credential material.
   This is a requirement for future daemon-state work, not present functionality.
   Credential use/recovery remains unselected; no status-only authority is assumed.
3. Browser may show multiple server-eligible workspaces. It is not told by native
   which one must win. It captures one exact current mapping at consent.
4. Native exchanges its own original launch tuple+code+verifier with the saved
   exact Multies endpoint. It never includes a guessed workspace or displayed
   tuple. This is the corrected current API, not a new code path.
5. Validate returned binding by exact equality against ONE tuple in both the
   saved allowed snapshot AND current independently installed configuration.
   Current config must still recognize the original origin/issuer and snapshot
   revision. Missing/changed config fails closed. Do not accept a new registrar
   merely because it is HTTPS or named by the response.
6. Persist the chosen exact tuple in the same pending intent before any registrar
   request. Retries/restart use that one selection only. They cannot pick another
   workspace, network, audience, revision or registrar on a timeout. The issued
   ticket is never accepted by native as node authority.

If the browser chooses a tuple native does not allow, local setup stops. The
one-shot issued ticket ages out; native does not contact the untrusted registrar,
automatically retry consent, overwrite its identity or silently choose another
workspace. A fresh user-started attempt can use newly provisioned trusted config.
Re-reading a known local config is not authority to acquire new remote config.

Tradeoff: strict snapshot equality may reject a harmless operator update. That
availability cost is intentional for this proposed first implementation. Dynamic
trust discovery/rotation and user-facing config onboarding need their own design.
No product addresses or mapping values are selected here.

## Registrar service authentication, proposed shape

Prefer the narrow existing primitive for the first internal integration:
a separately provisioned random256bit bearer secret over authenticated TLS,
with configured public key ID, exact service identity, issuer, audience,
registrar origin and active/revoked state. This is a proposal for review, not
an implemented or selected production protocol. Never generate real credentials
in a fixture or ask anyone to paste them into conversation.

Multies stores only the secret digest in private service config; registrar must
hold the real secret privately. A public digest is never a usable authentication
factor. Fixed-size digest comparison is constant-time after strict token parsing.
Use Authorization header only, never URL/query/body/cookie, no redirects and no
inherited ambient authentication. Ticket is a separate body field. Reject Cookie
and Origin on this server surface. Authenticate exact service scope before any
ticket lookup; generic failures do not reveal whether a ticket or reserve exists.
Bound body size, strict duplicate-aware JSON, exact fields and no secrets in logs.

Key rotation is explicit operator config: each key ID maps to one fixed service
identity and a currently allowed digest. Removal rejects new calls. Never allow
a client-supplied service ID, issuer or registrar URL to select authority. Revoke
versus rotate semantics for in-flight reservations must be recorded explicitly.
Proposed conservative rule: revoked identity cannot reserve or reconcile; a
rotated active key for the SAME identity may read/ack the same existing reserve.
It cannot alter that reserve. Whether privileged operator reconciliation is
needed after identity revocation remains an open operational design item.

This is service-to-Multies authentication, NOT native status/recovery auth. The
proposed node-control credential's capabilities and transport are not selected.
Do not reuse service bearer keys for node APIs, native clients or proof signing.

## Reserve, status and acknowledgement, proposed exact semantics

All three endpoints remain unimplemented, independently off by default and
local-development only until separately reviewed. Current issued tickets still
have no consumer. No current schema is silently upgraded into service authority.

### Reserve

Authenticate configured service first. Request carries ticket, native attempt,
opaque reserveId and exact descriptor containing version/purpose, handoff ID,
service identity, original native launch bindings and final selected mapping.
Immutable person/session/workspace provenance is taken from the Multies journal,
not asserted by registrar as trusted facts. Exact all-field equality is required
where the registrar supplies an echo; the conditional transition pins a canonical
descriptor digest and schema/encoding version. No raw JSON serialization hash.

Only an unused, unexpired issued ticket with current eligibility, exact operator
mapping and correct service identity can transition to reserved. One reserve and
commit descriptor per ticket/attempt permanently. The update must correlate live
eligibility in its authoritative statement; a prior SELECT is insufficient.
Concurrent different descriptors yield one winner. Identical authenticated retry
returns the same bounded reservation metadata, never ticket plaintext or a new
deadline. A consumed/uncertain/expired/withdrawn record cannot create a reserve.

Reservation deadline is a required explicit fixture input, not a production
number. Service retries never extend it. Before Store commit registrar rechecks
current configured tuple/service identity and authoritative reservation status.
This is NOT atomic with a subsequent independent Store commit. The residual
check-to-commit window must be bounded by reviewed lease semantics and revocation
handling before activation. Adjacent requests alone do not establish a guarantee.

### Status

Authenticated exact same service identity plus handoff/attempt/reserve/descriptor
identifiers returns only bounded state, pinned deadline and receipt identity if
recorded. No plaintext ticket, native credential or unrelated provenance. Service
status is not a native receipt endpoint and does not authenticate native callers.
No matching record is not evidence that a possibly committed Store node is absent.
Config/eligibility change must distinguish permission to create NEW authority
from permission to record already-committed historical evidence.

### Ack and uncertainty

A trusted registrar can claim a receipt; service authentication proves which
registrar reported it, not independent proof its disk committed. Verify exact
same reserve/attempt/descriptor and receipt ID/digest. Store durability remains
that registrar's responsibility with later failure-injection evidence.

reserved -> consumed records the matching committed receipt idempotently.
reserved -> uncertain follows deadline/ambiguous commit status and never resets.
uncertain -> consumed records matching late commit evidence ONLY. It cannot
extend lease, renew eligibility, reissue a ticket, create another reserve or
reactivate a revoked node. consumed retries must match the identical receipt;
changed receipt/descriptor refuses. No ack can transition an unreserved ticket.

Revoked membership/session/config does not erase reservation evidence, nor
prove no Store commit exists. Refuse new commit authorization while retaining
uncertainty. An authenticated same-service late ack can record historical commit
without validating that node for current access. A revoked service identity
cannot self-reconcile under this proposal. Offline/operator recovery remains
separate. Neither timeout nor withdrawal licenses automatic fresh enrollment of
a key that may already have committed elsewhere.

## Audit ordering and retention

Current handoff has no organization audit events. Do not claim the existing
preview audit call supplies this missing boundary. A separate audit write after
mutation may fail after code/ticket issuance; a write before mutation may falsely
claim a grant. Retrying a one-shot secret response is not a repair mechanism.

Proposed design: transactionally persist bounded nonsecret events/outbox entries
with each authoritative state mutation. One candidate for the initial journal is
an append-only per-record event sequence stored in the same atomic row update
as the new status. Never overwrite the previous undelivered event with a single
latest-event field. Include a monotonic sequence and unique event ID, and refuse
new authority transitions before write if event capacity is exhausted. Historical
late-ack recording needs separately reserved capacity so pressure cannot silently
discard uncertainty evidence. The exact representation, append bounds and
reconciliation-capacity strategy must be reviewed before schema implementation.
Native secret plaintext is never included. An idempotent projector exports to the org audit
sink keyed by that immutable event ID, retrying delivery without renewing grants.
If current audit sink cannot enforce event-ID uniqueness, add a reviewed sink
change before claiming exactly-once visible audit. No assumption of distributed
transaction support on SQLite/D1. Batch/transaction and conditional-write behavior
need actual runtime verification before choosing implementation syntax/schema.

Audit payload should name actor/service, target record, operation, mapping
revision, allowed/declined/expired/uncertain/committed result and timestamps, not
code/ticket/verifier/private credential or full launch URL. Deleted workspaces
need durable security-event handling without recreating an org or cascading the
receipt. Exported provenance/retention needs a product privacy decision; the
current terminal scrub does not also scrub a future separate audit destination.

Future reserved/uncertain rows must be excluded from current issued-ticket expiry
and terminal scrubbing logic until reconciliation/retention is explicitly designed.
Never prune them because a consent session was deleted or a timeout passed.
Hash tombstones count against explicit bounded fixture capacity. Global permanent
capacity exhaustion is documented, not a workable production retention policy.
No90day default, irreversible deletion policy or arbitrary garbage collection is
selected by this design.

## Launch URL privacy, attempt squatting and limiter behavior

Current URL carries full key, public digest, attempt/challenge/state. It may be
saved in history/logs; no-referrer/no-store does not remove history or screenshots.
An eligible session seeing it can squat the globally unique attempt before the
owner, forcing restart, but cannot satisfy the native verifier. Do not describe
that as prevented by PKCE. Do not put tickets or private secrets in launch URLs.

A future launch-channel redesign could use a native-authenticated pre-registration
plus short-lived opaque browser handle. That still needs anti-squatting/auth proof,
privacy and storage review; moving metadata to another unauthenticated endpoint
merely moves the DoS. No such route or possession proof is added here.

Current common handoff limit20/min/IP counts reads and at least four ordinary
requests, so same-IP reloads can starve a one-shot exchange. Proposed internal
next fixture separates cheap invalid-service auth, authenticated service reserve,
status/ack retries and native exchange budgets. Per-service limits do not replace
per-user capacity, byte limits or a global unauthenticated abuse ceiling. A native
exchange without cookies has no trusted user ID until code verification. Neither
IP-only fairness nor public attempt ID is an authentication mechanism. No secret
or full ticket may become a rate-limit log key. Thresholds, burst/retry limits and
shared-state behavior must be explicit fixture settings and measured, not copied
into production. Starvation errors never consume/reset/extend consent authority.

## Reviewable implementation and acceptance gates

1. Review this design and resolve native trust provisioning, ack-under-revocation
   rules and audit transaction/outbox feasibility. No code inferred from approval
   of earlier journal corrections.
2. Implement service-auth parsing/config in isolation, default off/development
   only. Test wrong/missing/duplicate headers, exact origin/service/key ID scope,
   revoked key, rotation, logs/redaction and auth-before-ticket-lookup.
3. Implement reviewed journal reserve/status/ack and atomic audit envelope with
   real local Multies fixture. Test concurrent same/different descriptors,
   one-winner reserve, deadline boundaries, response loss, changed mapping/session,
   disabled feature, infrastructure faults and late exact ack with no new grant.
4. Require actual file DB reopen plus D1/runtime transaction/variable/capacity
   evidence, idempotent audit sink behavior and private retention checks. Existing
   Vitest assertions/source reading do not supply those guarantees.
5. Only then consider Store atomic commit, daemon durable intent and reviewed
   key-possession descriptor/adapter. Need single-writer/directory-fsync/post-rename
   uncertainty semantics, tombstone/legacy-rejoin isolation and credential protocol.
6. Exact installed local native/browser/registrar journey, then physical two-host
   network/revocation/OS acceptance. Each tool retains separate network, screen
   and input permissions; release/live configuration requires separate authority.

No service calls, credentials, schema migrations or tests were executed for this
design-only change. Evidence for actual existing behavior is recorded in the
journal checkpoint/correction documents, not replaced by this proposed design.
