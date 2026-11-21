# Bounded status/ack implementation plan following reserve review

Status: proposed concrete next slice, not implemented or activated. Baseline
Multies7bb2b03b, Mnet49a5f946. Parent requested a consolidated execution plan,
not another unbounded design cycle. The independent reserve review executed
Python descriptor verification and SQLite full-chain/rebuild/reserve/rollback
reproductions, but not our Vitest, HTTP, Miniflare or typecheck/lint suite.

## New D1 parser evidence, no repository migration edits

Disposable two-database Miniflare/workerd experiment, same installed runtime and
compatibility date2026-10-26:

- ORIGINAL unmodified migration list fails at0073 after68applied migrations with
  the same D1 no-statement error.
- VARIANT changes only the in-memory copy of0073 statement index9 from its trailing
  double semicolon to one semicolon. All158migrations then apply in that disposable
  emulator database.

This supports the reviewer's double-semicolon hypothesis for this installed
emulator. No SQL source, generated migration, driver, production DB or deployment
was changed. Full unmodified D1 chain is still FAILED. The variant's successful
SQL application is not a passed application/HTTP/remote-D1 deployment test. A
separately authorized narrow migration hygiene fix, reproduction regression and
full unchanged-after-fix migration run are needed before closing that blocker.

## Selected engineering proposal for the bounded next slice

Add local service status and acknowledgement only after parent accepts this plan.
No Store/node commit, daemon credential, key-possession call, native adapter,
org-audit projector, external credential or production change. Preserve exact
service identity/origin/audience/mapping and all current development-only gates.

### 1. One migration, compatibility and event capacity

0175 rebuilds the table CHECK to add uncertain/consumed, with receiptId,
receiptDigest, descriptorEncodingVersion and monotonic eventSeq. Preserve all
existing rows, indexes, reserve IDs, descriptor/deadline and seq1event. No preview
conversion or secret redelivery. Validate copied events against the expected
existing reserve sequence; refuse malformed legacy state rather than inventing
commit evidence. Migration executes atomically under the existing driver.

Use the existing maximum8events but assign an explicit small finite lifecycle:
reserve(seq1), uncertainty(seq2when needed), commit-ack(next seq). No event for
reads, retry, auth failure or repeated identical ack. New reservations require
capacity for all three possible immutable lifecycle events up front, not just
one free slot. eventCapacity becomes min3for enabled service configuration;
startup refuses old fixture values below3rather than silently expanding them.
Existing already-reserved rows retain physical CHECK capacity8and their seq1;
terminal bookkeeping must use this preserved capacity even if a later operator
reduces a new-admission setting. No capacity extension through retries.

Record mutation plus JSON append plus eventSeq increment in the SAME conditional
UPDATE. SQL binds next seq to current eventSeq+1; JSON length must equal eventSeq;
WHERE enforces capacity and legal predecessor state. Audit IDs are random unique,
payloads nonsecret and bounded. A conditional loser rereads the exact immutable
winner; never appends a duplicate event. Still no org-audit delivery claim.

### 2. Deadline and explicit uncertainty

A separately named helper can change reserved->uncertain at now>=reserveDeadline
with one uncertainty event, conditional status guard and reserved event capacity.
It never withdraws/erases the reservation or ticket hash or changes its tuple.
Call it on authenticated service status/ack and bounded local maintenance paths;
request-driven execution is not a clock-time guarantee or background monitor.
Status must never report a new-authority-valid reserve beyond the deadline even
if the stored row has not yet been transitioned. Repeat status adds no event.
Pre-reservation sweep remains unchanged and cannot scrub reserved/uncertain rows.

No event/state policy may infer failure of a remote Store commit from timeout.
There is still no Store commit in this slice. The transition records uncertainty,
not proof of non-enrollment, automatic renewal or fresh-consent eligibility.

### 3. Status and ack are service bookkeeping, not native authority

Both use exact authenticated service identity and strict handoff/attempt/reserve/
descriptor identifiers. No ticket redelivery or native receipt endpoint. Status
returns state/deadline/receipt metadata only, not private native material or full
person provenance. No arbitrary URL is returned/followed. Unknown and wrong-scope
requests share generic refusal, preserving history unchanged.

Ack adds strict canonical receiptId and receiptDigest to those immutable fields.
They are a trusted registrar's CLAIM of durable commit, not independent disk
proof. No real commit receipt is created by a native or browser success flag.
Matching reserved->consumed may append one commit event before deadline. If the
deadline has passed, first record uncertainty then match uncertain->consumed;
required capacity is pre-reserved. Both statements individually atomic; a crash
between them leaves explicit uncertainty, never a fresh reserve. Consider direct
late-ack consumed with an uncertainty marker only if review prefers fewer writes;
do not silently change the chosen semantics during implementation.

Consumed identical receipt retry returns the same stored result with no event.
Changed receiptId/digest/descriptor/reserve/service refuses. No ack of an issued,
pending or unreserved record, no new token, no deadline extension and no node
activation. Never reuse validateCurrentRow's pre-reservation withdrawal helper.

### 4. User revocation versus service revocation

Reserve/new authority continues to require current live person/session/member/
workspace/operator mapping. Status/late historical ack on an already pinned
reservation must NOT require that deleted membership still exists. It must
require the same currently active service identity with exact pinned issuer/
registrar/audience scope. Thus user deletion can deny future authority without
suppressing evidence of a commit that might already have happened.

Revoked service identity cannot call status/ack or self-reconcile. Rotated active
key for the SAME immutable service identity/scope may report that same record.
Changed service scope cannot adopt it. No automatic admin recovery route is added;
privileged offline reconciliation remains unresolved and explicitly held.
Changing current workspace mapping cannot rewrite the pinned historical tuple.
Reports must distinguish recorded commit from currently permitted network use.

### 5. Service budgets

Keep current separate IP abuse ceiling and inherited global app300/min guard,
with no bypass. Add explicit fixture per-authenticated-service limit/burst fields
in private config. Authenticate BEFORE incrementing the service-specific budget;
key on configured serviceId, not raw token or public attempt. Shared DB counters
for both budgets. Status retries/ack need explicit separately measured fixtures,
no invented production defaults. Rate-limit refusal never consumes/renews authority
or edits deadline/history. Global/IP starvation remains a known limitation, not
solved by this additional dimension. No custom Cloudflare binding guarantee.

### 6. Distributed authority window stays unimplemented

Neither status nor ack authorizes Store commit by itself. Current config checks
and remote SQL cannot atomically commit a separate registrar's Store. A future
bounded authorization lease, durable revocation and pre/post-commit race/failure
injection must precede a consumer. This slice must not add a generic allowed=true
response that a caller could mistake for timeless permission. Deadline/history
alone does not solve offline peers, legacy reusable-key rejoin or revoke policy.

## Acceptance work, exact source and failure boundaries

- Failing-first actual service API tests for deadline boundary/status/late ack,
  then fixture-only implementation and strict body/header/auth-before-lookup.
- SQL migration preservation/fault rollback, copy seq1, reject invalid JSON/
  sequences/receipt conflicts, no cascade and full event-capacity behavior.
- Concurrent status+late ack, same/different receipt, exact retry, rotation,
  revoked user versus revoked service, mapping changes and disabled config.
- Trigger/error injection before each authoritative event append: no state-only
  success. Crash/reopen between uncertainty and consumed retains history. Full
  retention sweep leaves reserved/uncertain/consumed untouched until a separately
  approved privacy/reconciliation policy exists.
- Actual file-backed DB close/reopen and local Miniflare SQL for new migration,
  event UPDATE/RETURNING/concurrency semantics. Unmodified full-chain D1 failure
  remains recorded separately until source fix is authorized and verified.
- Measure IP/service/app budget stacking with separate identity and same-IP
  cases, no token logging, bounded requests and no credential disclosure.
- Build exact review packet and report implemented states versus held consumers.
  Existing tests run only where relevant source changed. No visual/native claim
  from server fixtures. Native trust provision, proof/Store/node activation,
  org-audit sink, physical two-host work and release remain outside this slice.

A status/ack implementation is proposed, not already approved by this document.
The current runtime remains reserve-only until the parent accepts a bounded plan
and original owner scope supports it. No forward authority comes from review.
