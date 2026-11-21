# S25 sticky close and scoped final source gate

S23 once-only inert suite: 134 pass/3 fail. S24 once-only inert suite:
138 pass/1 fail. Both red logs retained by independent reviewer, no builder
reruns or reconstruction. S24's remaining failure revealed a production-model
custody bug, not another fixture issue. All S25 changes are uncompiled/untested.

## Exact failure path

Stop chooses close-owned-tun-fd, CloseOriginalFD returns an error after modeled
link removal, then absent readback permits the loop to continue. Its next empty
Rollback path calls closeFDOnce. Old fdClosed meant attempted, but returned nil
on repeated call even after error. Stop then cleared PendingCleanup. This silently
converted observed network absence into FD custody confirmation.

## Correction transition table

| Prior close state | Event | Result |
| --- | --- | --- |
| not attempted | original handle close returns nil | attempt recorded, confirmed only for close call; absence still required |
| not attempted | original handle close returns error | original error retained, PendingCleanup true; absent readback cannot clear it |
| failed/ambiguous | later Stop | same retained error, PendingCleanup true, no further close or forward work |
| failed/ambiguous | raw reconcile equal | network absence observed, close ambiguous/custody unresolved still true |
| failed/ambiguous | raw reconcile differs | unresolved absence and custody |

fdCloseAttempted and fdCloseError replace misleading fdClosed. Every subsequent
closeFDOnce returns the retained error, never retries. Stop short-circuits failed
custody before any further cleanup. Current-call close error is returned before
absent-readback can become cleanup success. Independent read-only reconciliation
is unchanged and never mutates Transaction custody. No reset, retry or adoption.

## Final identity scope

Adapter construction now requires explicit nonempty target/session identifiers
and the same isolated namespace as the captured baseline. Its owned final record
carries a value copy of that identity. Capability projection refuses final-scope-
mismatch before readiness if target/session/namespace differ from witness scope.
A clean Final from session A cannot establish custody state for session B. Identity
is supplied to this pure model, not authenticated by it. No production constructor
caller exists. Historical poison can be resolved within its own bound session
only with confirmed close/raw absence and separately valid supplied witnesses;
no historical record creates or renews permission.

## Independent test-source changes, not run

The failed-close test drives actual Start/Stop through the inert fake, repeats
Stop twice and requires sticky original fdCloseError, pending custody and exactly
one fake close. Existing independently decoded raw-baseline/mutated-MTU assertions
remain. Positive final/witness projection now uses the same explicit bound session,
not new-explicit-session. A separate table mutates final target, session and
namespace while keeping otherwise valid witnesses, requiring scope refusal.

Production-model delta is transaction.go only. No live tags/bodies/device/namespace
or defaults changed. Exact S24→S25 diff is included at packet root for independent
review. No compile/vet/tests/mutants or real effects for this revision.
