# Session binding and durable consumed-proof storage checkpoint

Isolated public-data/storage engineering after0fdffdff review. No key-possession
verifier, challenge issuer, permit constructor, atomic authority-creating Store
transaction, service client/route, credential verifier or native consumer exists.
This supersedes raw session-ID suggestions in the earlier protocol direction.
Existing reserve-v1 and receipt-v1 bytes are unchanged. No Multies edits here.

## Settled session-binding contract for the future two responses

Use one independent CSPRNG256bit `sessionBinding` generated when NEW handoff row
is inserted. Canonical nonzero unpadded base64url32bytes. It is a public opaque
per-handoff pseudonym, NOT a bearer token, internal sessions.id, sessions.token,
plain hash of either, secret-derived HMAC or user identifier. Original row keeps
its real private userId/sessionId for current eligibility checks. The binding is
associated with that exact original session by the authoritative row insertion,
not by a publicly checkable hash relationship. New handoff means fresh independent
random bytes even for the same person/session. No salt/key lifecycle is needed
because no value is derived from a low-entropy internal ID. Random generation
and row association still require source verification; syntax does not prove
entropy, source ownership or a correct binding.

Threat model: a recipient seeing this value cannot dictionary-test raw session
IDs or join different handoffs by this value alone. It does NOT hide links from
the originating Multies server, a database reader, or other shared identifiers.
Principal ID, raw device key, receipt and network remain linkable; retaining those
requires its own retention/privacy policy. This is pseudonym minimization, not
anonymity or encryption. Recording the same sessionBinding in the native PKCE
response and registrar evidence intentionally correlates THAT one handoff.

Exact future Multies change: add a nullable session_binding column for existing
rows without backfill; new versioned evidence-capable handoffs insert random
binding atomically alongside original user/session. Existing rows lacking it
remain ineligible for NEW v4 proof/evidence, never synthesize it on read/retry.
A separately versioned native code+PKCE response and authenticated exact live
reserve-evidence response both return the SAME row binding and explicit
principalAuthority/namespace/localPrincipalId. Never expose raw session IDs.
Raw internal session remains used ONLY server-side for liveEligibility. Existing
historical status/ack do not acquire these fields or become live grants. Gate
all changes to the already default-off development-only enrollment feature.
Strict parser/config/privacy/auth/response-loss/current-session tests and actual
SQLite/D1 evidence required before claims. No schema migration or response change
has been executed by this checkpoint.

Because this changes semantic field meaning, the inactive codec moved to NEW
`mnet/enrollment-key-possession/experimental-v4` domain and new golden files.
No fallback from session_id/v3. Field8 is canonical sessionBinding as43UTF8
base64url characters (not raw internal session bytes). Other26field order is
unchanged from v3. Authority and issuerLabel CAN have identical URL bytes; explicit
namespace/config, not URL inequality, distinguishes local principal semantics.
A regression test permits equal bytes and rejects raw/empty/zero session binding.

## Exact proposed DH/MAC construction, still not implemented authority

Future crypto review target, not a security certification or live algorithm:

- Device private key: existing durable WireGuard X25519 private identity, never
  rotated/exported into browser. Device public32bytes must equal independent
  presaved intent plus current trusted mapping. Crypto implementation must check
  valid X25519 input/low-order rejection and fail on ECDH error/all-zero output.
- Registrar: independent CSPRNG X25519 ephemeral key per accepted challenge,
  separate from service bearer credential and node control secret. Store private
  challenge owner retains ephemeral secret only in bounded memory, never in
  receipt/durable transcript. Public ephemeral is bound by exact v4 transcript.
- `shared = X25519(registrar_ephemeral_private, device_public)` at registrar,
  `shared = X25519(device_private, registrar_ephemeral_public)` at native.
- `K = HKDF-SHA256(ikm=shared, salt=raw32byte nonce,
  info=UTF8(domain + "/derived-mac-key"), length=32)`.
- `response = HMAC-SHA256(K, exact Encode(v4Challenge))`, raw32bytes. Constant-time
  MAC comparison. Transcript includes challenge ID, nonce, ephemeral public,
  explicitly named principal/session binding, service/mapping, native attempt,
  reserve descriptor+version, full device public key, native credential digest
  and ALL deadlines. No MAC over request JSON, no secret/digest as bearer.
- Exact expected binding must come from native's own authenticated PKCE response
  plus presaved credential/key/current independently installed authority config,
  never challenge.Binding copied back. Registrar expectation comes from its
  authenticated evidence source, not caller body. Neither DH nor MAC proves a
  person currently eligible; authenticated service evidence and lease semantics
  remain independent preconditions. Trusted registrar already has network power.
- Bad MAC/scope/transcript does not consume a legitimate challenge. Comparison
  and state transition under same owner prevent concurrent winner duplication.
  Clear ephemeral/key material best effort; do not claim secure-memory erasure.

No production function computes this MAC or manufactures a proof/permit here.
Old keypossessionv2 remains an explicitly inactive, different-domain experiment.

## Bounded challenge admission, specified before an issuer

Future challenge creation ONLY after verified exact live evidence and matching
current config; never make public IDs/digests an authentication factor. Required
fixture capacities: global live, per principalAuthority+localPrincipalId live,
per handoff/reserve live, and bounded byte size. No defaults/product limits.
One exact live binding reuses its existing challenge, without new nonce/key or
extended deadline. Conflicting same-reserve request refuses without replacement.
Capacity-full refuses NEW admission; never evicts an unexpired legitimate
challenge or resets its deadline to make room. Expired entries can be removed
only after expiry is established conservatively under clock assumptions; capacity
for durable consumed history is separate and cannot prune tombstones.

Unauthenticated/invalid proof attempts must not allocate challenge slots.
Separate cheap pre-auth abuse budget and authenticated service/principal admission
rate budget are needed to prevent generation work becoming a DoS. Identical
retry/status still bounded. Repeated wrong proof does not evict/consume original;
per-source throttling may delay responses without changing challenge authority.
A compromised trusted principal can exhaust its explicitly bounded share, which
is an availability residual rather than a reason for LRU evictions. No challenge
issuer/admission cache has been installed in this checkpoint.

## Implemented strict registrar storage v2, not a transaction

`registrar_state.version=2` adds REQUIRED `consumed_proofs` map INSIDE the strict
versioned namespace, not a droppable legacy top-level field. Empty stays explicit
on unrelated saves. State v1 remains supported unchanged and rejects new fields.
State v2 supports old receipt-v1 records without proof claims and new receipt-v2
records with REQUIRED `consumption` containing exact public v4 challenge and
transcript_digest. Missing/null/unknown/duplicate/case aliases fail strict load.
Existing receipt-v1 history is not upgraded into evidence or a consumed proof.

Consumed index key is canonical JSON [principalAuthority, registrarOrigin,
challengeId], value is permanent canonical network/raw-key claim key. Both
references are checked in both directions, no missing/dangling/extra index or
same proof assigned to another record. Transcript SHA256 base64url must match
actual v4 bytes. Stored transcript device raw key/digest/network/tuple/service/
attempt/reserve/descriptor/access deadline must exactly match record. Commit time
must be inside that claimed proof window. These are consistency checks on trusted
local storage, not cryptographic validation or evidence authenticity.

Receipt-v2 domain `mnet/store-enrollment-receipt/v2`, fields1through19 same order
and encoding as v1, but with v2 domain; then field20 exact challengeId UTF8
base64url and field21 exact transcriptDigest UTF8 base64url. Each receives uint32BE
byte length. Transcript digest binds all named provenance/session/deadlines and
proof public inputs; no private DH key, response or native credential is stored.
Phase remains mutable and excluded so expiry/delete preserve exact receipt.
Old receipt-v1 domain/19field vector remains byte-for-byte unchanged.

All new proof-consumption objects are PRIVATE data types, not exported permit
issuers. No production creator or transaction method exists. Actual Store fixture
setup is test-only. Load/save now enforce this v2 consistency through existing
single owner/persistence. Revocation/delete/unrelated saves/reopen retain index,
public challenge and exact immutable receipt. Public hashes remain forgeable to
a local file writer; they are not signatures or external authorization.

## Migration and rollback gate

No automatic v1->v2 migration or runtime flag is added. A future first v2-producing
transaction must explicitly initialize the index from zero only when moving a
strictly validated v1 store; existing v1 records stay v1, never invent consumed
proofs. This requires a separate reviewed connected transaction batch.
The original actual-Store failing-first v2 test demonstrates previous17ff code
rejects version2 instead of dropping its index. Current version1 parser rejects
v2 consumption fields in v1 records; simply relabeling version cannot form a
supported downgrade. Frozen/closed lifecycle blocks writes as before.

Supported rollback after any v2 commit must use a binary that understands v2;
rollback to pre-v2 code requires refusing startup, not silently stripping proof
fields or restoring an old snapshot while external reservations may have committed.
Even older pre-namespace binaries ignore/destroy history, so deployment rollback
needs an external operator/build compatibility gate. The new parser cannot make
an old binary safe. No live rollout/restore process is implemented or exercised.

## Evidence and remaining boundary

Actual Store test baseline first failed on unsupported v2; now passes with empty
consumed index and retained v1 history through an unrelated save. Actual v2
fixture tests cover revive/delete/reopen retention, malformed index/transcript/
principal/session/deadline/downgrade/mixed-version refusal without rewriting.
Independent Python struct-packed v4 transcript and receipt-v2 goldens match actual
Go encoders, alongside unchanged receipt-v1 vector. No second fake registrar.

Durable transaction remains future: side-effect-free reviewed MAC verification,
private authenticated evidence permit, exact history-before-new-authority check,
one candidate image for consumedProof+node/IP/digest+receipt+claim, all persistence
fault stages and concurrent/restart losers. Final pre/postpublication freshness
and conservative accessUntil<=reserveDeadline direction remains a FIXTURE bound,
not immediate upstream revocation or production connectivity policy. Existing
clock assumptions/direct-peer map expiry/native expected binding+presaved secret/
actual restart/OS/D1/daemon-save gaps stay open. Product lease/retention/recovery
policy and network/screen/input authority remain separate and unselected.

Executed validation: ten focused race repetitions for actual consumed-history,
receipt and codec/clock contracts passed; full Linux go test ./... passed with
26 packages; affected vet and diff checks passed; Windows amd64/macOS arm64
control tests crosscompile, not native execution. Python --check matches both
receipt versions and v4 transcript. Golden v4 is26fields/718bytes/SHA256 hex
`10c048e678a8f7cb584e078297be8c0443a8acd1bd7bc676219e8b7374733b4b`.
Golden receipt-v2 is21fields/669bytes/SHA256 hex
`2f4c05771ffbbe086c35ee7a30723a43081ee37bc1889b3883a3523567da85c7`.
The earlier v3 transcript vector is deliberately retired from the current tree,
not silently given new session semantics under the same domain. Its old committed
692byte evidence remains historical. Reserve570byte and receipt-v1568byte vectors
are unchanged. New consumed-history prepublish rollback/postpublish uncertainty
and strict nested duplicate/alias tests pass on real Store files/reopen.
