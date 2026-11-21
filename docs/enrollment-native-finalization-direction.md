# Native finalization and recovery, actual remaining contract

Design only after owned preparation/source DATA integration and local RD integration. No finalize API, State schema change, node credential writer, route, bootstrap scheme, lease selection or activation is added. This document is not approval of a protocol or product policy.

## Current source-grounded blockers

Native State in internal/daemon/state.go has PrivateKey/PublicKey plus legacy Server/Network/NodeID/NodeToken/NodeName/AuthKey. Its strict native JSON loader has no registrar credential kind, receipt, transcript, epoch, access expiry or finalization intent. Ordinary State.Save deliberately preserves existing key and exact loaded revision; the actual daemon owner serializes publication and freezes uncertain writes. Merely adding fields to a detached DTO is neither enrollment permission nor a checked writer.

Actual daemon join in internal/daemon/daemon.go obtains NodeID/NodeToken from legacy Join and builds controlclient.Client with Token. That client sends X-Node-ID/X-Node-Token. Store registrar validation forbids mixed NodeTokens for registrar records and its legacy credential path denies registrar-controlled nodes. The pre-saved native secret cannot be installed into NodeToken to bridge this gap. That would silently reinterpret a credential and still would not authenticate against the current registrar Store. A separate explicit native credential kind, versioned storage and dedicated authenticated control transport/verifier are missing.

Pending journal experimental record2 owns original full policy, native secret/verifier, original launch, callback code and exchanged-data. It contains no outstanding registrar challenge/MAC/confirmation, qualified receipt or finalization marker. Existing exchanged-data means strict own-PKCE source DATA was saved, never enrolled or current network authority. Marking it complete from DH confirmation would be false.

Registrar receipt-v2 storage binds exact node/network/device/credential digest/scope/expiry and consumed transcript digest. The existing receipt serializer/validator is private control storage logic. There is no authenticated wire receipt delivery/history contract for native, and no actual commit entrypoint atomically consuming proof and publishing node/claim/credential/receipt. A successful service reservation with authorityGranted:false is not that receipt. No finalization consumer can honestly exist until those sources exist.

## Required separately reviewed boundaries

1. Explicit native credential-kind schema, including refusal of mixed legacy token/auth-key with registrar identity. Preserve legacy bytes/behavior; no implicit migration. Older binary downgrade discipline remains required because unknown JSON fields alone cannot prevent an old executable misusing familiar fields. New credential must not be placed in a field an old binary understands as a legacy token.
2. Actual authenticated exact-registrar channel/bootstrap, independently owned source evidence, full expected binding and source time/lease semantics. Public digest, ticket, parsed transcript or crypto object cannot be a permit constructor. Wire parsing and DH arithmetic are already separate DATA/crypto prerequisites, not this authority.
3. Actual Store transaction: proof verification side-effect-free until one candidate publishes consumed proof, immutable claim, node, digest credential and qualified receipt atomically. Prepublish retry uses SAME proof; unknown publication freezes and owning reopen resolves history. Historical retry cannot renew current access or allocate another node.
4. Authenticated same-registrar immutable receipt/history response and dedicated current-control credential verifier/client, with exact qualified receipt matching, expiry/revocation denial and no legacy token fallback.
5. Native-owned finalization transaction/recovery with one durable credential location and explicit cross-file outcomes. No source acknowledgement or lost response may cause rotation/reconsent or an alternate key/network.

## Two local files are not one transaction

Native State and pending journal have distinct file owners and persistence. Each individually uses private temp/full-write/sync/rename/parent-sync and uncertainty freezes. Two adjacent successful saves are not atomic together. A protocol must identify which durable image is authoritative at every crash point, and avoid creating two writable secret copies interpreted as separate authority.

A candidate future ordering, not implemented or approved, is a write-ahead pending finalization record followed by native State publication, followed by historical journal completion. The pending record would pin exact original key/policy revision, selected mapping/provenance, outstanding proof identity and independently authenticated qualified receipt, credential digest/kind, target State revision and exact finalization identity. It must NOT be treated as an alternate usable node credential. Actual native State remains the sole usable credential image. Secret transfer/storage handling needs explicit version design; an ordinary getter is not a solution.

| Crash or failure point | Required recovery invariant |
|---|---|
| Before authenticated receipt | No native credential; preserve pending outcome, query only same exact owned attempt when authorized |
| Receipt observed, pending finalization save fails before publish | No State change; receipt observation alone creates no local usable authority |
| Pending finalization rename/sync ambiguous | Freeze local finalization, no State write, owning reopen inspect whole pending image |
| Pending marker durable, State not published | Exact same original key/target/receipt may be reconciled; never generate a fresh credential or call legacy Join |
| State publication rollback-safe failure | Old State remains, pending marker retained; retry exact candidate only after current owner/policy/receipt checks |
| State publication ambiguous | Freeze native identity operations, do not clear pending marker or roll back in memory and continue |
| State durable, journal completion absent/ambiguous | New State's explicit finalization identity must correlate exact pending record; do not enroll again or rotate secret |
| Journal says completed but State absent/different | Refuse usable enrollment, preserve evidence; no reconstruction from an arbitrary returned receipt |
| Expired/revoked historical receipt after restart | History may be recorded, never creates current access; separate verifier refuses current operations |
| Legacy enrolled State or conflicting registrar claim already exists | No silent identity migration/takeover; explicit recovery decision remains required |

Reconciliation must use actual retained owners/current exact installed policy and actual disk images, not cached DTO values. Current policy changes may stop new authority while preserving historical evidence. Close must cancel network work and dispose handles before either owner releases. No lock may be held across remote I/O. Exact lock order and cancellation must be tested with actual concurrent State/journal/Close callers.

## Acceptance before enabling anything

A future implementation needs failing-first tests of its actual selected API/schema, not a test-only proof=true/trusted permit constructor or a fabricated receipt. Exercise every temp/write/sync/close/rename/parent-sync failure, crashes between the local files and after server commit, timeout/current-policy/key changes around publication, concurrent duplicate/conflicting proofs, owning reopen, same receipt/history replay, expired/revoked access and old-binary refusal. Native source integration and Store source integration already execute real local producer/DB paths but do not cover these missing finalization semantics.

No product clock uncertainty, challenge lifetime, source observation age, access lease, retention/GC or re-enrollment epoch is selected here. Current fixture arithmetic cannot supply these policies. Installer/browser/native OS/physical two-host acceptance also remain separate. Network, Remote Desktop view, pointer control and Shared Desk input retain distinct authority.

## Current strict-loader compatibility baseline

Test-only enrollment_schema_boundary_test.go now executes actual owning LoadState/New/detached Save/fresh EnsureKey against a real durable native key plus unknown future credential markers. Marker names in the test are deliberately hypothetical, not a selected new schema. Every current-owner path refuses without rewriting exact State bytes, adding alternate credential/pending files or rotating keys. Restoring original test bytes verifies failed opens released actual stable owner and preserved the same key. Separate actual-owner tests reject partial familiar legacy node/token/target/key combinations rather than treating a node ID with absent legacy token as registrar enrollment.

This is current strict-loader refusal acceptance only. It does not test an older executable. Historical permissive decoders may ignore unknown markers, so merely adding a new namespace cannot establish safe downgrade. New schema must avoid filling legacy NodeToken/AuthKey with registrar material; installer/version-floor and supported actual old-binary rollback tests remain unresolved. No future-schema loader/writer, secret transport or finalization action is added by this baseline. Proof that current code rejects an unknown marker is not proof that a future schema is correct or that a historical binary refuses it.
