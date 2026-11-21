# S28 source and UNRUN tests: owner/deadline and raw gate

No compile/vet/test/mutant execution. Single-owner, sequential in-memory model
only, no locks/concurrency claim, journal/restart recovery or authentication.
Transaction.Stop sets stopStarted immediately before first read, invalidating
active-display eligibility even before cleanup effects. The session binds one
exact Transaction pointer whose Kernel is this inert adapter, exact Plan/handle,
Ready true/no stop/close, and live wall AND monotonic owner deadline. A different
Transaction, expired owner or rollback of either sampled clock refuses. Clock
rollback poisons ownership permanently. No caller-ready boolean or new deadline.
Projection rechecks this owner through the ledger. Side effects here are solely
in-memory distrust bookkeeping under the same sequential-owner contract.

observeActive accepts ONLY RawInventory+SysctlSnapshot composed by the captured
baseline, not arbitrary modeled Inventory. It validates the exact handle and all
forward readbacks. Mismatch poisoning is intentional conservative policy, never
an automatic setup retry. Observation times cannot go backward; shared sequences
must advance exactly once. Final+Active is an explicit conflicting-records refusal.
Caller CleanupPending remains custody-unresolved on every branch.

## Newly discovered source blocker, deliberately NOT bypassed

policy_inventory.go Compose calls validateForwarding on every raw link. That
calls LinkIPv6Applicable requiring attr26 AF_SPEC. Compose later allows owned
attrs only3/4/18/20 and refuses26. An owned raw link therefore cannot currently
pass both checks. This contradiction was found by source inspection, no runtime
probe. No allowlist widening or guessed AF_SPEC policy was performed. Positive
raw active observation/readiness remains blocked pending a separately reviewed
owned-link AF_SPEC schema correction. Inert owner eligibility is not that proof.

## Test source changes, UNRUN

Construction fixtures now include explicit attempt identity. Historical resolved
poison+clean final is explicitly NEVER ready, including after24h, with fresh
matching witnesses. Foreign target/session/namespace/attempt final refuses.
Actual failed-close Evidence result asserted exactly once, later Stop no new
entry/call. Tests cover raw AF_SPEC contradiction as a refused poisoned gate,
Transaction stop/deadline/clock/different-owner eligibility, historical sequence
replay/rollback/overflow, foreign-cleanup caller flag and empty AttemptID.

There is deliberately no manufactured positive active readiness fixture. The
strict raw path cannot currently construct one. Future sequence/expiry/full raw
positive projection tests depend on resolving that source gate. Earlier S25 green
is not validation of this revision. Existing test source adjustments do not mean
any test was run. No live/default/adapter activation or effects were added.
