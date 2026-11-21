# Journal-owned experimental workflow-v1 native channel

This is an earlier boundary checkpoint. The later actual sole-State continuation and current remaining gaps are documented in enrollment-native-sole-state.md.

Connected development continuation after fbb268a/Multies52d5655, whose independent review was still pending when this work began. This batch does NOT broaden readiness or production approval. No actual daemon durable-key caller, sole-native-State credential writer, deployment or screen/input authority is added.

## Separate wire, independent context

Existing experimental-v4 ID-only bootstrap client/journal-v3 remains unchanged in purpose and wire contract. New workflow-v1 uses exact configured HTTPS registrar origin, dedicated `Mnet-Enrollment-Workflow-V1 ` Authorization scheme and separate `/api/mnet-enrollment-workflow-experimental-v1/begin` and `/submit` paths. These routes are NOT registered by production Server. Test bridges are explicitly unregistered test code.

Begin sends version1 plus original SAME private source ticket/handoff, journal-chosen reserve ID, saved launch and selected mapping. This supplies the missing context that the old ID-only bootstrap could not send. The presaved secret is a dedicated Authorization header only, not JSON, URL, cookie or log. New transport has normal TLS validation, no proxy/cookie jar/redirect, no HTTP2/keepalive, no body replay, bounded request/response/header/timeout and no automatic retry. Close cancels outstanding I/O and drops reachable private inputs. Secret strings cannot be promised zeroized; constructor reconstruction alone is not a durable replay boundary.

New client accepts independently supplied own-PKCE source context/configured service. Returned challenge must match ALL independently saved principal authority/namespace/ID, per-handoff session binding, issuer/registrar/audience/service/workspace/network/revision, handoff/attempt/reserve, recomputed descriptor, durable device public key and presaved credential digest. It never copies response identity into expected identity. Only registrar-selected checkedAt/reserve deadline/access-until are admitted as bounded DATA after explicit native maximum source age, reserve lifetime, challenge lifetime, lease and clock-margin checks. That does not turn a source reserve into a source-issued access grant. Server-side initial development lease residuals remain as previously documented.

Submit sends version1, SAME attempt/challenge ID and raw MAC only. Client owns one begin and one submit slot, spent before request, no retry/refund. It accepts only full exact receipt-v2 projection of its own pinned challenge plus matching canonical receipt digest and bounded strict confirmation field. Exact-origin verified TLS authenticates the trusted registrar delivery, not the public receipt codec. Native proof owner must still independently verify confirmation MAC; confirmation alone is not a receipt or permission. Historical receipt delivery after an in-flight commit is DATA, not a current-access flag.

## Actual pending file is the replay boundary

`BeginWorkflowDATA` on actual Journal admits ONLY original live non-reopened record2/exchanged-data, SAME current original policy/principal/service/mapping. It chooses a fresh reserve ID ONCE and writes record4/workflow-begin-may-have-started to the SAME retained file before any begin network I/O. Record4 contains exact reserve ID/start/explicit limits and empty challenge/response/receipt. Only after actual verified channel + independent checks does it durably pin full ChallengeWire as workflow-challenge-data before returning challenge DATA. Current policy, own exchange, caller cancellation, original journal deadline and local monotonic/wall time are rechecked after I/O and after durable pin. A policy loss after pin retains history and returns ErrNoLongerCurrent, not a different attempt.

`SubmitWorkflowDATA` invokes NO private key. Caller provides only exact pinned challenge ID plus response MAC. Current own policy/time must pass; it durably writes workflow-submit-may-have-started with SHA256 of response bytes before sending. Raw response MAC is not retained as a second credential-like field. Client verifies and Journal durably saves the exact ReceiptWire as workflow-receipt-data before returning DATA. Current checks repeat after save. It never says enrolled, installs a credential, clears intent, starts a tunnel or accepts crypto confirmation as durable Store proof.

The Journal retains its one runtime WorkflowClient privately between the operations, not a client/secret getter. Close cancels client and outstanding context before releasing journal ownership. Reopened record4 reports only retained phase, never reconstructs a client or resumes network automatically. Lost begin/submit response, failed challenge/receipt pin and postpublication uncertainty remain SAME spent/unknown original intent. They do not reconsent, rotate key/secret, choose another reserve/mapping or assume a remote commit is absent. Explicit authenticated SAME-attempt history/restart reconciliation remains to be implemented, not simulated by automatic retries.

Existing record2 and record3 encodings OMIT workflow_json and are not silently migrated. Their strict decoder rejects even an extra empty workflow_json. New record4 requires its full exact payload/phase and rejects an explicit bootstrap_json field, avoiding cross-purpose ambiguity. Old accepted strict decoders do not know workflow_json and reject it. This source-level fact is NOT old executable/installer downgrade or version-floor acceptance. A malformed/corrupt file must not be repaired into usable intent.

## Failure outcomes

| Stage | Required outcome |
|---|---|
| Invalid own context/current policy/limits | No intent change or registrar disclosure |
| Begin marker rollback-safe failure | Original record2 unchanged, zero sends |
| Begin marker postrename/dirsync uncertainty | Frozen owner, zero sends; reopen finds whole begin-may-have-started |
| Begin loss or invalid/mismatched challenge | Begin marker retained, remote unknown; no returned challenge |
| Challenge pin rollback-safe failure | Begin marker retained, remote unknown |
| Challenge pin postrename uncertainty | Frozen/no return; reopen finds whole pinned challenge |
| Submit marker rollback-safe failure | Pinned challenge retained, no submission; same live owner can retry exact operation only within original bounds |
| Submit marker postrename uncertainty | Frozen/no submit; reopen finds whole submit-may-have-started |
| Submit loss or failed receipt save | Original spent submit marker retained; remote commit may exist |
| Receipt pin postrename uncertainty | Frozen/no return; reopen finds whole exact receipt DATA |
| Close or restart | No automatic network/key/State publication; preserve original secret/attempt/phase |

Rollback-safe filesystem failures do not falsely consume an unsent network request, but do not grant a new attempt. Any affected marker/result whose rename may have published freezes. No copy of the pending credential is installed as usable authority.

## Actual connected evidence

The opt-in actual Multies workflow fixture now uses the ACTUAL pending Journal for its second approved peer. Independently configured policy exists before its launch. Real journal Begin owns saved secret/PKCE; actual source ExchangeSaved performs the real own-PKCE-v2 handler/SQLite exchange. New BeginWorkflowDATA and SubmitWorkflowDATA make real verified TLS calls to an unregistered TEST bridge, which strictly decodes workflow-v1 and forwards to actual private Store begin/submit. That workflow makes real authenticated source reserve, owns DH challenge and atomically publishes actual node/claim/consumed-proof/receipt. Journal pins receipt before return. Fixture-owned disposable native proof verifies confirmation. Both actual current credentials see the other approved peer; deletion removes peer/current access while history remains.

No server-returned principal is treated as native expectation: peer identity comes from own-PKCE and explicit policy; descriptor is recomputed. Journal chooses and pins reserve ID. Native fixture separately rechecks full challenge via the independent-context helper before passing its disposable test key. This is not an actual daemon key caller or sole-State finalization. It does not start physical WireGuard/remote desktop/input transport.

TLS evidence is precise: cross-app source uses a fresh openssl-generated disposable local certificate. Registrar test bridge uses Go httptest's STANDARD embedded localhost certificate, not a fresh per-run CA. Both are merged into isolated fixture-only roots before first TLS use, with normal verification and no insecure flag. No production trust or live source is changed.

Focused journal tests use normal constructor/TLS in a fresh process and inspect the actual on-disk SAME marker BEFORE responding. Cover successful durable pin/receipt, wrong own principal/identity/timing, lost begin/submit, policy loss, Close, no replay/reopen and old byte shape. Full create/write/short/sync/close/rename/postrename/dirsync matrix at ALL FOUR begin marker/challenge pin/submit marker/receipt pin boundaries proves zero-send vs unknown and whole-image owning reopen phase. New strict wire tests cover exact purpose/version/nested shape, wrong receipt projection, duplicate/unknown/missing/case/fraction/null/oversize/invalid UTF8 failures. Public codec consistency is never labeled authentication.

## Remaining connected outcome

Actual daemon key admission must consume SAME owned journal channel/policy, not exported Challenge DATA or a public equality check alone. Actual native credential-kind/State publication must be sole usable image and correlate one exact finalization identity with pending receipt across two file owners, including every crash/fault stage. No raw secret getter, legacy NodeToken substitution, fake proof=true constructor or disconnected DTO can close that gap. Exact authenticated historical receipt/reconciliation after response loss must keep the SAME attempt/reserve/key/secret and never renew authority. Those are the next genuine native integration targets.

Production ingress/cost/abuse and historical disclosure policy remain blocked, including O(history) Store validation and per-process ephemeral admission budgets. Production leases/account-event revocation/renewal, provisioning, old-binary/version floor, full D1 runtime and physical/native OS acceptance remain separate. No production default or new user obligation is inferred.
