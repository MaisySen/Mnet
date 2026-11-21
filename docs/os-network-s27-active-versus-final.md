# S27 source-only active versus historical final

S26 evidence-once source accepted; S25 inert141 green remains the last runtime
validation. No S27 tests/source updates to tests, compile, vet or effects. Existing
capability fixtures remain intentionally incompatible pending contract review.

## Semantic correction

A closed-clean attempt is torn down. It NEVER means NetworkingReady, even with
same-attempt witnesses claiming os-path/transport/authority. Historical final and
active-session observation are now separate records. Supplying Final always
takes the historical-only branch, even if Active is also supplied. A dated clean
closed Final reports session-closed-clean, no readiness, no expiry; AsOf is its
original raw comparison time. Close-error/baseline-equal remains unresolved.

An active observation can only be obtained through observeActive from a complete
ledger: canonical bound handle, all six phases complete, no poison/cleanup/
pending/retry/close/rejected handle, nonempty attempt identity. It requires fresh
supplied Inventory exactHandle validation plus every forward operation present.
That Inventory is a caller claim in this inert model, not kernel proof. It then
returns an opaque observation with the ledger's identity, current sequence,
observation time and original Plan expiry. Failure of actual inventory matching
poisons setup. Poisoned/cleaned/closed attempts cannot create active observations.

Projection checks the owning ledger still meets eligibility and its sequence is
still current. Cleanup/close/poison therefore invalidates an old active observation
immediately in this in-memory model. It also requires matching claimed attempt,
expected sequence, valid observation interval and original Plan expiry, then the
separate same-attempt deployment/OS-path/transport/authority witnesses. Its display
validity is the earliest of these expiries. No input extends original expiry.

One-second freshness is now named provisionalActiveObservationFreshness and
applies ONLY to active observations, pending policy review. It is not a product
promise or authority lifetime. Historical cleanup is dated, never one-second-
expiring. Current time must be trusted; hidden monotonic stamps are removed for
wall-time projection, while Transaction retains actual monotonic enforcement.

## Sequence ownership and claims

The session owns observationSequence shared by active and historical observations.
nextObservationSequence returns the next expected number, zero on overflow.
Historical observed reconcile requires exactly that next value, no jumps/reuse;
active observation increments internally. Projection requires the caller's expected
current sequence and checks the ledger for active observations. History has no
restart/replay-proof store. Attempt uniqueness, time and supplied witnesses remain
caller claims. Empty AttemptID now fails construction. No ID allocator, auth,
journal/restart recovery, production supplier or real adapter is added.

## Conservative mapping

Caller CleanupPending initializes CustodyUnresolved and is retained on EVERY
return, including foreign-final and invalid-observation branches. A foreign
unresolved final is labeled foreign-attempt-cleanup-unresolved, not silently
reclassified as this attempt's cleanup; local caller pending still remains true.
A matching unresolved final ORs in its own custody state. No historical final
can fall through to active readiness.

## Independent future oracle plan, no test changes/run yet

Invert S25 positive closed-clean-ready fixture: same-attempt os-path witnesses
cannot make torn-down final ready. A positive live case must drive complete
Start plus observeActive, with no Final supplied; invalidate by cleanup, poison,
close, later observation sequence, Plan expiry, foreign identity and witness
expiry. Test exact next sequence/reuse/overflow; empty AttemptID construction;
foreign pending record plus caller cleanup flag; old dated clean final remains
historical after one second, never readiness. Verify failed close result event
once, repeated Stop no added event/call. Separate raw historical observations
from fake active Inventories. No kernel success inferred from any of these tests.
