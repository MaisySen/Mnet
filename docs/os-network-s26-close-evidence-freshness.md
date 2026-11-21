# S26 source-only close evidence and reconciliation freshness

S25 independent once-only inert suite141 green is the accepted prior revision.
S23/S24 reds remain preserved. S26 has NOT been compiled, vetted or tested. No new
test/mutant source, runtime effects or journal/restart recovery were added.

## Failed-close evidence

closeFDOnce now emits one close-owned-tun-fd-result Evidence event at the actual
failed call return, ACK false/Error true/Readback custody-unresolved. It also sets
PendingCleanup. Every repeated Stop returns the original error before recording
anything. It cannot fabricate another attempt/result. Successful-close evidence
continues through the existing readback path. Event order truthfully records a
failed call before any later network-absence observation.

## Reconciliation observation contract

Identity now includes caller-supplied AttemptID. Target/session/namespace remain
bound, but correlation IDs and witnesses are caller claims, not authenticated
identity or OS proof. Caller must mint a unique ID per owner attempt and never
reuse it. This library does not allocate IDs or persist/recover them.

Legacy reconcile still produces undated read-only diagnostic state. It can
report baseline equality and close ambiguity, but cannot make readiness true.
New reconcileObserved requires the exact attempt identity, increasing nonzero
sequence, explicit observation time and expiry. It stamps both equal and unequal
raw observations. The sequence is consumed for validly formed observations even
if raw comparison fails, preventing that sequence being reassigned as success.

Projection requires exact final/observation/witness attempt identity and exact
caller expected sequence. It rejects undated, stale, future, mismatched or invalid
intervals. A conservative source-proposed display freshness cap is one second;
this is NOT a product requirement or release decision, and should be reviewed.
Validity is the earliest reconciliation/witness expiry. A new observation requires
another raw comparison; supplied future expiry never renews permission.
Transaction still owns monotonic authority. Trusted current time and unique ID/
expected sequence supply remain caller obligations, not proven by this helper.
No production caller exists, so no readiness activation is implied.

## Status table

| Evidence | Status | Readiness |
| --- | --- | --- |
| missing final record | custody-unverified, unresolved true | false |
| matching attempt, unresolved FD/cleanup | custody-unresolved, unresolved true | false |
| foreign attempt with unresolved cleanup | foreign-attempt-cleanup-unresolved, not attributed to local custody | false |
| foreign clean final | final-scope-mismatch | false |
| matching clean final, stale/absent observation | reconciliation-stale-or-unbound | false |
| matching recent final, all fresh bound witnesses | specified path only, bounded by earliest expiry | eligible |

Historical runner denial remains a separate string observation, never current
readiness or a user's target check. Full journal and restart ambiguity recovery
remain unimplemented. No authentication or replay-proof cross-process guarantee
is invented here; caller claims can still lie about time/identity. Sequence and
freshness checks only reject internally inconsistent/stale supplied records.

## Next independent oracle plan, NOT implemented or run

One failed actual fake close creates exactly one failed result Evidence entry;
repeated Stop preserves entry count/error/one call. Raw-equal reconcile after
failed close remains unresolved. Different attempt/session/target/namespace
cannot use final or observation. Old sequence, missing observation, future time,
expired and over-cap interval refuse; earliest witness/observation expiry wins.
Foreign unresolved cleanup is never labeled local cleanup but blocks readiness.

Existing S25 capability test fixtures deliberately lack new attempt/observation
fields and will need source updates after this contract review. They are not
claimed passing on S26. No test changes or runs were made under this gate.
