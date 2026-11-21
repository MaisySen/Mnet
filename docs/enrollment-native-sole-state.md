# Actual sole native State finalization, inactive development

This follows independently scoped accepted native-key batch 2d78397 and Multies 9bd4ea9. That review does not accept this new delta. No production caller, native startup network trigger, registrar ingress registration, installed-device activation, tunnel, remote desktop or shared-input authority is added.

## Connected result

The third actual daemon process in the opt-in Multies fixture now continues beyond proof DATA. Actual retained native key/Journal, real Multies approval/SQLite/own-PKCE, real verified TLS workflow and actual atomic Store commit produce a qualified receipt. The SAME actual Journal confirms its outstanding proof engine, the actual daemon checks original native owner/policy, pending finalization identity is synced, and a private registrar credential is published in the existing sole `state.json`. Pending historical completion is then synced.

That daemon closes and reopens its actual owner. It checks exact State/pending correlation, sends its own endpoint update through the dedicated registrar client, fetches a real current Store map containing both other approved peers and observes the real endpoint update. Native key and registrar secret are never exported to the coordinator. The expected service receives the credential in its dedicated Authorization header, not a URL/body/legacy token. No legacy client or physical tunnel starts. The actual Store still proves mutual three-peer maps, deletion denial and immutable history, then retains three revoked records with zero live nodes or legacy tokens. Actual source rows remain reserved, no source acknowledgement is invented.

The old proof-only `nativeStateUnchanged` fixture flag is renamed `nativeProofStateUnchanged`, explicitly describing the checkpoint before finalization. Separate `nativeStateFinalized` and `nativeCurrentPeers:2` assertions describe what happens next. Finalization does change sole State.

## No public DATA-to-secret constructor

Existing accepted `ProveWorkflowDATA` and `SubmitWorkflowDATA` remain DATA-only. They do not create finalization readiness, even when the MAC is valid and the exact receipt is delivered on the real channel. Passing a matching public receipt to the new finalization method cannot expose the journal's credential.

`ProveWorkflowOwned` is a distinct composition. Actual daemon code still checks the original retained native owner, key, State bytes, policy and preparation under `d.mu`, then consumes the SAME opaque admission under native-to-Journal lock order. `RespondProof` returns the actual outstanding crypto engine, not a verification boolean. Journal owns that engine, submits its response through the original marker/channel, verifies the authenticated exact receipt confirmation MAC against that engine itself and closes it on every result. Only a successful exact confirmation plus current own policy/time/channel/receipt creates private runtime readiness tied to the exact Journal image. No parser, equality helper, arbitrary document, boolean or reopened file can create that readiness.

The daemon repeats current native checks before retaining its confirmed continuation. `finalizeNativeEnrollment` has no document/secret/receipt parameter. It accepts only cancellation context and consumes that SAME preparation. Journal `FinalizeWorkflow` additionally requires its private runtime readiness and exact receipt pin. The only credential transfer is the narrow actual-native publication callback while actual native and Journal locks are held. This is internal composition, not an externally available getter or a new owner grant.

## Explicit owner-only State representation

New private on-disk namespace `native_enrollment_v1` is an encoded strict JSON envelope. It contains the exact `NativeCredentialDocument` version/kind/secret/finalization identity/full challenge/qualified receipt/digest and original native-State byte revision plus exact original policy byte revision. The envelope is private to the retained native owner, not an exported field on `State`. Actual owner serialization and loading explicitly handle it. The document is validated recursively with the existing strict contract decoder; the envelope separately refuses unknown/missing/duplicate/case/null/invalid fields.

Legacy encoding is unchanged when no native credential exists. Legacy State/key/metadata behavior remains covered by full regression. Native kind cannot mix NodeID, NodeToken, AuthKey, Server, Network, exit/subnet/exit-use flags or a missing/mismatched durable key. The registrar secret is never put in NodeToken or AuthKey. Historical expired native documents may be loaded as history, but that does not authorize current requests.

`State()` and `LoadState()` return a redacted detached snapshot with no registrar document or secret. Ordinary JSON serialization of a snapshot omits the namespace. A private detached marker prevents `Save` from stripping/replacing a native credential; ordinary owner save also refuses removal/change of an existing private credential. `cloneState` deep-copies the private document owner. Existing WireGuard private-key behavior of public State is unchanged; this change adds no new registrar-secret getter. Mutable temporary proof arrays are cleared, not a promise that Go strings/copies are zeroized.

Legacy `Up`, mobile `Prepare`, `join` and routing paths refuse the native kind before any legacy enrollment request or State mutation. New/New State loading does not start a registrar client or network operation. Its only new pending safety read prevents an absent sole State from generating a replacement key when any earlier pending intent survives, and refuses a finalization marker paired with only legacy State. It preserves files rather than reconstructing authority. Old record2/3/4 pending shapes otherwise remain unchanged and accepted under their original semantics.

## Two files, explicit crash states

A new strict pending record5 adds `finalization_json` containing one random identity, original State/policy revisions and exact receipt digest. It retains original key/secret/attempt/source/receipt context as historical pending intent, never an alternate usable credential. Existing record2/3/4 omit the new field and reject an explicit extra field. Record5 forbids cross-purpose bootstrap data and requires a full valid workflow receipt plus marker. Older source decoders refuse the new version/phase/field; no actual old executable/version-floor acceptance is claimed.

Ordering:

1. SAME runtime proof confirmation and original current owner/policy are required.
2. Sync pending `native-finalization-prepared` with exact identity before State publication.
3. Under actual native owner, publish the private credential in the existing sole State. Recheck original State/policy/cancellation immediately before rename. Recheck original policy/cancellation/lease after sync; possible publication then freezes native owner. Wall time and local elapsed time bound this operation.
4. Sync pending `native-finalization-completed` only after actual sole State publication and current native check.
5. Only exact completed correlation can make a dedicated current request.

| Failure/crash | Outcome |
|---|---|
| Prepare-marker rollback-safe failure | No State callback; original receipt pin/readiness retained for same live exact retry |
| Prepare-marker publication uncertain | Frozen Journal, no State write; reopen preserves whole prepared image |
| State rollback-safe failure | Original State unchanged; SAME live confirmed owner can retry the exact prepared identity/document, not generate a new one |
| State rename/sync or postpublication-policy/time uncertain | Native owner frozen; no current operations; reopen reads whole native image or original image |
| State durable, pending completion absent/rollback-safe/uncertain | Native image exists but current requests refuse until exact historical correlation completes under both actual owners |
| Native State plus exact prepared history after reopen | Explicit private `reconcileNativeFinalization` may complete only local historical pending bookkeeping; no State rewrite, source call, proof replay or renewal |
| Pending finalization with absent/legacy State after restart | Owning loaders refuse; no key generation/legacy fallback/reconstruction. Authenticated SAME-attempt history recovery still needed |
| Completed pending with different/missing State, wrong finalization identity/revisions/secret/receipt or missing/replaced pending | No usable native current operation; preserve evidence |
| Expired receipt | History may be completed, current client refuses; never renews original access |

The exact pending file is reread and compared to the retained complete canonical image during correlation, not only a cached DTO. Current native State checks also verify actual State bytes, private owner, original policy byte hash and binding. Filesystem protections do not isolate against an adversarial same-UID filesystem writer. Two adjacent saves are not called atomic together.

## Dedicated private current operations

Private native map and endpoint methods require actual sole State, exact completed SAME pending correlation, original installed policy and bounded dedicated-client admission before disclosure, then repeat owner/policy/correlation/expiry checks after I/O. The dedicated registrar client is retained per actual native State revision, sharing finite request/concurrency budgets and permanent-denial state across operations. Close/state uncertainty cancels/disposes it. A 403 is not legacy unauthorized and cannot initiate legacy rejoin. No automatic restart loop, relay, routing capability or production map endpoint is added. After process restart budgets are process-local, not durable abuse controls.

## Evidence

Actual native tests run in a fresh process with normal verified TLS. They perform real source exchange, actual saved-key DH, actual engine confirmation, finalization through real State owner, native create/write/short/sync/close/rename/postrename/directory-sync faults, same-live exact-ID retry, policy loss before/during/after State publication, expiry during temp preparation and after rename, actual owning reopen and missing-State/completed-history refusal. Dedicated client tests cover exact own map, wrong own key, policy loss in flight, permanent denial/no second send/no legacy fallback and Close cancellation reaching the TLS server. Strict native schema negatives exercise actual LoadState/New without rewrite. Public snapshots/JSON/detached saves, all legacy paths and no client/tunnel after proof/finalization are checked.

Journal tests use real test-owned crypto engines and normal TLS, then inspect the actual prepared file before the State sink. Both prepare/completion boundaries exercise full create/write/short/sync/close/rename/postrename/directory-sync faults and whole owning reopen phases. DATA-only valid MAC/authenticated receipt cannot create readiness or transfer a credential. Exact mismatched finalization/document/original revisions refuse. Completion recovery never invokes a missing State sink. Earlier begin/key/submit/receipt fault matrices remain.

The actual cross-app six-case regression includes real third-daemon finalization, native Close/reopen, real current Store endpoint/map and actual two approved peers, plus original native source success/decline/revoked/lost-response. Test-only registrar bridge remains unregistered, uses standard embedded httptest localhost certificate, and the real source uses a fresh disposable certificate. Both are validated against isolated fixture roots. No live account/service/device is involved.

## Remaining work and limits

Authenticated SAME-attempt remote history recovery remains unbuilt for begin/submit response loss or pending-only restart. This delta deliberately refuses to publish absent native authority from parsed receipt history. Installed native UI/first-run policy provisioning, actual native OS/physical two-host/direct WireGuard acceptance, production client activation, old executable/version-floor controls and D1 runtime remain separate.

Production ingress remains held for O(history) validation AND whole-state MarshalIndent under Store mutex, cost/abuse/historical disclosure policy and ephemeral process budgets. Development registrar access is still an explicit finite registrar lease, not a source-issued grant or production duration/renewal/revocation policy. Removal/restoration of original local config can preserve a bounded previously committed lease after owning reopen; this is not immediate source/account revocation. This delta does not choose production leases, renewal, provisioning, account-event revocation or broaden network/screen/input permissions.
