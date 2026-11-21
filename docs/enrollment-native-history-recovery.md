# Immutable same-attempt native history recovery, inactive development

This follows independently scoped accepted sole-State batch c71483c and Multies 3b6e9f3. That review does not accept this new recovery delta. No production registrar route/native trigger, startup networking, installed device, tunnel, remote desktop or input activation is added.

## Connected result and exact scope

A new opt-in cross-app `history-recovery` case performs real Multies approval/SQLite/own-PKCE, actual retained daemon key proof and actual atomic Store commit for the third device. The unregistered test bridge deliberately drops the response AFTER the real commit. Actual native Journal stays `workflow-submit-may-have-started`, without accepted receipt or confirmation. The daemon closes and reopens its actual existing State and Journal, waits past the explicit fixture challenge/access lifetimes, then performs one separate authenticated immutable-history lookup. The bridge deletes that node before calling the actual Store historical method. The native owner durably pins the exact retained receipt DATA, with unchanged State/key/secret and no current authority.

That case has exactly three source exchanges and three source reserves, the original three devices only. No recovery source exchange/reserve, submit retry, reconsent, new reserve/key/secret or mapping occurs. The fixture never claims the lost response confirmed the proof: `nativeOwnedKeyConfirmed` is false for this case. `nativeProofStateUnchanged` remains true and `nativeHistoryRecovered` is true; `nativeStateFinalized` is false and `nativeCurrentPeers` is zero. Expired current credentials remain denied. Normal success still performs the previous actual sole-State finalization/reopen/endpoints/map with both peers; source denial still creates no node. Original four native source cases remain.

This batch deliberately returns historical DATA only. It does not create finalization readiness, a confirmation MAC, current access, a native credential or State. The record5 prepared-finalization/legacy-State loader conflict remains held rather than being bypassed. Pending-only finalization recovery is a separate remaining outcome. Begin-response loss has no full pinned challenge and is refused, never interpreted as evidence that reserve/commit did not happen.

## Distinct channel and DATA grammar

New history-v1 uses exact configured HTTPS registrar origin and separate `Mnet-Enrollment-History-V1 ` header purpose at `/api/mnet-enrollment-history-experimental-v1/receipt`. These are not registered in production Server. Existing workflow-v1 begin/submit and older experimental-v4 ID-only bootstrap are unchanged.

The request carries version1 plus the independently pinned FULL original challenge, not a response-selected node/receipt reference. Original presaved secret is sent only in the dedicated Authorization header. Client validates its TEXT digest against the original challenge before any request. Public digest/grammar is not authentication or a permit. Actual Journal/native owner must establish original provenance/policy before disclosure.

Response is version1 plus exact immutable ReceiptV2 and digest, with NO current/enrolled/approved/renewal/confirmation field. Strict canonical projection must match every original challenge binding and committed transcript. When an earlier workflow receipt was already pinned, history must match that exact receipt/digest too, not another syntactically valid node/receipt projection. The normal verified TLS exact-origin service owns the returned historical fact; parser consistency alone does not prove the source.

The one-shot dedicated history client has no proxy/cookie jar/redirect, no ambient legacy headers, no HTTP2/keepalive/body replay or automatic retry, bounded body/headers/timeout and Close cancellation. It clears reachable inputs on Close but does not claim Go strings/copies are zeroized. Missing/denied/404/malformed/lost responses all remain unavailable/unknown. None proves commit absence or safe proof/source retry.

Expired original source/challenge/access does not prevent a READ-ONLY historical lookup. Source context is decoded at the original recorded observation solely to match identity, not refreshed or treated as current eligibility. Historical Store lookup remains actual private `registrarHistoricalReceipt` from the earlier independently accepted boundary: exact full challenge plus original secret, immutable consumed claim/receipt, no allocation/save/renewal/revival, works after expiration/revocation/deletion. Its O(history) full validation remains an ingress blocker.

## Native and pending ownership

Private `recoverNativeHistoryDATA` owns the actual existing native State key/revision and installed current policy before opening the existing Journal. It accepts only cancellation, explicit lookup timeout and finite maximum attempts, not caller challenge/key/secret/receipt. It validates the saved native public key against the original Journal launch. Before each current-policy callback it reacquires the native mutex and rechecks original State bytes/key/context. A different policy or mutated native image stops disclosure. No durable private-key operation is invoked. Actual daemon Close and state fault cancel in-flight lookup, including a separately reopened Journal.

No native lock is held across HTTP. Journal never invokes the native current callback under its own mutex. A single runtime history operation owns the phase; concurrency cannot reset the finite budget. Journal Close cancels/closes the dedicated history client before releasing its file lease.

New strict record6 adds `history_json`: original version/phase, fixed timeout, fixed max attempts, durable attempt count, last start and exact received history payload. Only original record4 submit-may-have-started or workflow-receipt-data is eligible. It requires the pinned challenge and response digest; challenge-only/key-may-have-started/begin-unknown, record5 finalization and completed native State correlation are not silently repurposed. Original workflow/finalization fields are preserved and validated under their original semantics.

Every explicit lookup commits `history-lookup-may-have-started` and increments its count BEFORE any credential disclosure. The initial maximum is explicit, one to four attempts, and timeout is an exact whole-millisecond value up to one minute. Both bounds persist unchanged. Reopen may make another explicit SAME immutable read only within that ORIGINAL budget, never refunding an unknown send or resetting it by changing parameters. There is no automatic retry. Marker rollback-safe failures do not spend a slot; ambiguous marker publication freezes and owning reopen discovers the whole image, conservatively spending it.

Current policy/cancellation/elapsed and wall clock are checked before marker, again after marker before HTTP, after response and after receipt pin. After successful verified-channel result, Journal commits `history-receipt-data` before returning DATA. Failed/late/policy-lost receipt pins preserve history or unknown state but return no accepted result. The original challenge/source expiry is not extended. Local lookup timeout bounds only that operation, not an access lease.

Record2/3/4/5 omit `history_json` and reject an explicit extra field. Record6 validates the original record4 image, exact full challenge/response digest, finite counters and separate strict receipt grammar; cross-purpose bootstrap fields are refused. Older source decoders reject version6. No actual old executable/version-floor acceptance is implied.

## Failure outcomes

| Boundary | Outcome |
|---|---|
| Wrong actual device/policy/image, malformed file, unsupported original phase or invalid bounds | No disclosure or marker rewrite |
| Marker rollback-safe fault | Original intent unchanged, zero HTTP, slot not spent |
| Marker postrename/parent-sync uncertainty | Frozen owner, zero HTTP; owning reopen sees whole spent marker |
| Policy/cancel/deadline loss after marker | Spent marker retained, zero HTTP |
| Missing/denied/lost/invalid response | Exact spent marker retained; remote history unknown, not commit absence |
| Receipt pin rollback-safe fault | Spent marker retained, no returned receipt |
| Receipt postrename/sync uncertainty | Frozen/no result; reopen sees whole exact history image |
| Policy change after receipt pin | Historical receipt retained, no accepted current result |
| Reopen | Same original secret/key/challenge/context and fixed finite budget, no automatic source/proof action |
| Exhausted budget | Zero additional sends, no reset by changing timeout/max or reopening |

## Evidence and retained limits

Focused Journal tests use normal verified TLS in a fresh process and deliberately expired original source/challenge/journal/access context. Both marker and receipt boundaries cover create/write/short/sync/close/rename/postrename/parent-sync faults and whole owning reopen. Actual file is inspected before serving history. Tests cover policy before/during marker/in flight/after pin, wrong native device, replaced actual file, Close, missing full challenge, wrong previously pinned receipt, concurrent callers and persistent budget across loss/reopen/parameter-change refusal. Transport tests cover exact full request/headers, no ambient authority, wrong origin/secret/TLS, redirect target zero requests, malformed/wrong receipt/cache/oversize/404/loss/Close/cancel/one-shot behavior. Strict wire rejects duplicate/unknown/missing/null/case/fraction/oversize/UTF8/cross-purpose input.

Actual native tests check reopened State ownership/key and policy before disclosure, mutated image refusal, unchanged sole State/no legacy client/tunnel, persistent budget and inability to finalize from history DATA. Actual cross-app history case proves real committed-then-lost proof, actual native reopen and expired/deleted actual Store receipt, not only a consistency fixture.

This is not end-to-end lost-response enrollment completion. Authenticated historical-delivery readiness, any recovery State sink and narrow record5 recovery owner remain unbuilt. Normal New/LoadState safety is not relaxed: corrupt/oversize/multilink pending can block even legacy owners, a conscious fail-closed availability limitation. Existing native mutex spanning both finalization file protocols remains a cost/availability boundary. Record6 is strict safety-validated by the native loader, but supplies historical DATA only, never native authority or a workaround for those gates.

Production ingress remains held for O(history) validation AND whole-state MarshalIndent under Store mutex, disclosure/cost/abuse policy and ephemeral process budgets. Production lease/renewal/account-event revocation/provisioning, old executable controls, full D1 runtime and installed/native OS/physical/direct WireGuard/desktop/input acceptance remain separate. Local policy removal/restoration/owning reopen can preserve an already committed finite lease; this is not immediate upstream revocation. Nothing here broadens activation or network/screen/input authority.
