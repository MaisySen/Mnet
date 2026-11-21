# Connected enrollment protocol and atomic-consumption direction

Isolated checkpoint after storage17ff62ed review. No live enrollment, service
call, Store transaction, permit constructor or proof verifier has been connected.
The new internal/enrollmentcontract package is public-data syntax/encoding and
explicit offline fixture-time arithmetic ONLY. Neither helper authenticates a
claim or grants authority. Old proof v2 remains untouched and unimported by
control/daemon. Existing vector contracts are not changed: current reserve
vector remains570bytes, current receipt vector568bytes, each its own encoding.

## Explicit principal identity, no OIDC alias

Proposed new enrollment evidence names:

- principalAuthority: exact independently installed Multies HTTPS origin.
- principalNamespace: literal `multies-local-user-v1`.
- principalId: authenticated journal.userId in that authority's local user table.
- sessionId: authenticated original journal.sessionId in that same authority.
- issuerLabel: exact current operator issuer configuration, retained as a mapping
  label. It is NOT evidence that principalId is an OIDC subject at that issuer.

This does not redefine Mnet OIDC account identity. No mapping into existing
ApprovalIdentities, NetworkMember issuer/subject, account sessions or identity
projection follows. A future product needing that mapping must provide verified
principal correspondence as a separate source. Native and registrar both need
new explicit config recognizing principalAuthority+namespace; old config issuer+
subject cannot silently opt in. Principal/session IDs are disclosed only through
a separately authenticated service evidence response, not launch URLs, browser
messages, native UI logs, general status or receipt recovery. A versioned native PKCE exchange would
return that same namespaced principal/session binding ONLY to the owner of the
code+saved verifier, so native can durably pin an independently authenticated
expected value before contacting registrar. Session identifier here is a
non-bearer protocol binding; never expose a session token or cookie value.

Current local Multies service authority is a trusted registrar, not an adversary
constrained by a public hash. TLS/auth keys must still be exact configured scope.
The provenance scheme names where authenticated internal identity came from;
it is not a claim of a new external OIDC/JWT assertion.

## Exact proposed Multies extension, not executed in this checkpoint

Inside the ALREADY isolated development-only service reserve flow:

1. New explicit fixture config fields principalNamespace and authorization lease
   bound, both validated at startup/runtime; no fallback from existing issuer.
2. A separately versioned authenticated reserve-evidence response, or a clearly
   new reserve response version, returns exact journal userId/sessionId using the
   named namespace, principalAuthority from explicit independent server config,
   the pinned descriptor/reserve tuple and source checkedAt/reserveDeadline.
   Never trust caller-provided person/session or accept email/display identity.
3. Evidence is emitted only for a successful NEW reserve or exact live reserve
   retry after its existing live-eligibility/current-mapping check. Historical
   status/ack stay historical, retain current revocation reporting behavior and
   NEVER acquire new live authorization fields.
4. The separate native enrollment PKCE exchange response version must return the
   SAME named principal-authority/namespace/principal/non-bearer-session tuple
   from its code-bound row. Native validates its installed authority+namespace,
   saves this with the selected mapping, then compares the future registrar
   challenge against that saved expected tuple. Do not source native expected
   principal solely from the registrar challenge. If raw internal session ID is
   inappropriate for disclosure, add a stable per-handoff session-binding digest
   in BOTH responses and the transcript before implementation, never substitute
   a session bearer token. This is an explicit protocol/privacy review item.
5. Revalidate active pinned service/tuple after async reads; conditional authority
   statement still checks current eligibility. Authenticate before lookup. Bound
   strict duplicate-aware shape, no cookies/origin, no redirects. Error/redaction,
   current vs deleted session, mapping change, namespace mismatch and response-
   loss tests required. Existing full D1 migration failure still blocks claiming
   deployed/full-runtime acceptance; this extension does not fix historical0073.

Rationale: registrar cannot populate independently expected principal/session
from the present reserve response; leaking historical status fields is neither
fresh consent nor a reason to widen service disclosure. These are precise future
changes, not a Multies edit or inferred permission to activate them.

## Expanded possession transcript v3, byte contract implemented offline

Domain `mnet/enrollment-key-possession/experimental-v3` with uint32BE length then
exact bytes for EACH field. Ordered26fields:

1. Domain UTF8.
2. Challenge ID, decoded32bytes.
3. Nonce, decoded32bytes.
4. Server ephemeral public key, decoded32bytes.
5. Principal authority UTF8.
6. Principal namespace UTF8.
7. Principal ID UTF8.
8. Original session ID UTF8.
9. Operator issuer label UTF8.
10. Registrar origin UTF8.
11. Audience UTF8.
12. Service ID UTF8.
13. Workspace ID UTF8.
14. Network ID UTF8.
15. Mapping revision UTF8.
16. Handoff ID, decoded32bytes.
17. Native attempt ID, decoded32bytes.
18. Reserve ID, decoded32bytes.
19. Exact reserve descriptor digest, decoded32bytes.
20. Descriptor encoding version, literal ASCII `1`.
21. Device public key, decoded32bytes.
22. Native presaved credential digest, decoded32bytes.
23. Evidence checkedAt, uint64BE Unix milliseconds.
24. Reserve deadline, uint64BE Unix milliseconds.
25. Access-until, uint64BE Unix milliseconds.
26. Proof expiry, uint64BE Unix milliseconds.

Encoding is not proof verification. IDs/digests use strict canonical nonzero
unpadded base64url32bytes as in current handoff. No normalization, JSON object
order, HTML escaping or newline enters bytes. Exact HTML/nonASCII/U+2028/U+2029
vector is Python struct-packed and compared to actual Go bytes. Every primitive
field is tested to change bytes or refuse. Old proof v2 cannot produce these
bytes and is not automatically upgraded; no v2 fallback is proposed.

Future crypto step may reuse the reviewed experimental X25519/HKDF/HMAC mechanism
with the v3 domain only, but needs its own cryptographic/protocol review. It is
not implemented by this codec. Native expected binding must originate from
presaved intent/current independent config plus authenticated exact service
context, never challenge.Binding echoed back. In particular native must recompute
its original credential digest and raw durable public key, then compare the
attempt/reserve/descriptor/deadlines before using its key. A public digest does
not prove possession of the secret or force a malicious trusted registrar to
obey its own rules. Scope of this proof remains key possession, not consent.

## Bounded local fixture semantics without false immediate revocation

Engineering direction for the FIRST disconnected fixture: require
checkedAt < accessUntil <= reserveDeadline and proofExpiry <= accessUntil.
No separate long-lived access grant can escape an expired reservation. Explicit
fixture MaxLease, MaxObservationAge and ClockUncertainty have no defaults.
Encoding rejects absent/contradictory bounds. Offline CheckFixtureTime rejects
future checkedAt/backward clock, stale observation, oversized lease, deadline
EQUALITY and uncertainty-margin equality. Subtraction avoids integer overflow.
Tests simulate pause/recheck, deadline and extreme integers.

These helpers use caller DATA. They are NOT a trusted clock, an authoritative
Multies response or a production authorization check. A trusted adapter would
need bounded server/local clock assumptions and recheck local monotonic elapsed time
in addition to wall time. Never persist a process monotonic reading as trusted
across restart. On restart, online fresh evidence is needed for NEW authority;
historical receipt can still be reported without renewed access.

This fixture deliberately permits only a short bounded initial access window,
not a useful production connectivity policy. It introduces NO renewal endpoint,
fixed production duration or automatic consent loop. Product-quality long-lived
network authority/renewal and account-event effects remain explicit later design.

Important residual: latest live eligibility and local commit cannot be atomic
across these two systems. Lease semantics mean revocation after checkedAt can
remain effective until bounded accessUntil; immediate upstream revocation is NOT
promised. Mapping/service removal in the registrar's own current config denies
NEW commits; existing leased access policy remains separately stated. Clock
accuracy assumptions and already distributed direct peer traffic prevent an
unqualified hard real-time revocation SLA. User/account/workspace revocation
expectations must be accepted before activating even a bounded lease design.

Why final-publish recheck matters: validate freshness under Store.mu immediately
before rename and after durable publication. A pause during rename can publish
late. Since accessUntil<=reserveDeadline and every authority read denies expired
access, late persistence can preserve historical consumption without being a new
usable access grant. If final check fails, freeze rather than return successful
network setup. Reopen may report exact immutable receipt, but must not extend
access or start a new attempt. This reasoning does NOT prove wall clock honesty,
direct-peer map expiry enforcement or production revocation, still unimplemented.

## Durable proof consumption in actual Store, proposed next transaction

DO NOT call current Pending.Verify, then Join+Approve. That consumes proof
outside durable state and splits node/receipt from authorization history.

Future owning control implementation keeps private challenge state indexed by
attempt/reserve under bounded fixture capacity. Issuance has no node/authority
side effect. Cryptographic verification is side-effect-free until Store commit.
A private verified service-evidence adapter is the ONLY producer of an immutable
permit; there is no exported/data-only constructor. Input fields alone never
manufacture trust. Permit binds exact tuple, descriptor, credential and principal
namespace, with an explicit proof identity and deadline. No network I/O occurs
while holding Store.mu; fresh evidence acquired outside is rechecked locally
inside but retains its stated distributed window.

Atomic candidate contains one node/IP/digest credential, exact proof challenge ID
and transcript digest, qualified immutable receipt and permanent key claim.
All indexes/history land in the SAME existing Store persistence publication. A
prepublish error restores candidate and leaves challenge unconsumed for exact
retry. Durable success consumes challenge and returns one receipt. Ambiguous
publish freezes the Store, returns no success and never discards consumed-history
candidate to permit a second attempt. Owning reopen checks record history FIRST.
An exact receipt retry reports immutable historical result and current-access
state, not a positive authority grant. Scope/key/descriptor/proof/credential
conflicts refuse before ID/IP/key-use/version changes.

On process restart, unpublished ephemeral challenges are deliberately invalid,
not replayable from a public transcript. If no durable receipt exists and live
exact reserve remains authorized, a fresh challenge for the SAME immutable
attempt/reserve may be issued, never a replacement attempt/ticket. If a receipt
exists, proof/history is consumed and no new challenge may grant again. Permanent
handoff/attempt/reserve and raw-key constraints must be checked under the SAME
Store lock before accepting either winner. Old responses cannot become new
permits after restart. Persist challenge ID/transcript digest in the new record
version, not the temporary server private key. This requires a schema/receipt v2
upgrade; current stored v1 was intentionally not overwritten in this checkpoint.

## Checkpoint and next acceptance

Implemented now: inactive v3 DATA codec, strict namespace/field/deadline syntax,
fixture-only time checks, executable independent Python+Go bytes and adversarial
mutation/pause/skew/boundary tests. No real principal mapping, verified evidence,
cryptographic verifier, Store proof record, credential route or consumer.

Before callable authority: review exact evidence route/config scope; implement
strict authenticated response source and native expected binding; reviewed v3
proof verifier; actual Store record/receipt v2 with durable consumed-proof index
and conflict/fault/concurrency/restart tests; final-boundary clock behavior; native
presaved secret and separate authentication transport; installed local journey.
Native OS, D1, daemon-save and physical/direct-mesh acceptance remain outstanding.
No deployment/activation or production access/retention/recovery policy is selected.


Validation for this checkpoint: Python --check matches the committed v3 vector,
26 fields/692 bytes/SHA256 hex
`08205d132df79fe3b0788e653d3390edf7037090a19cc5b643705b4cc4756485`.
Ten race-enabled package repetitions and go vet pass. The first fixture run
correctly rejected a principal ending in a Unicode separator as trailing
whitespace; fixture now ends with a literal suffix, preserving internal Unicode
separator coverage. No parser relaxation was made to fit the fixture. Existing
Store receipt/reserve codecs and proof v2 have not changed.
