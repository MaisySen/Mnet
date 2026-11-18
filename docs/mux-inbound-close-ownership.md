# Inbound stream table ownership on session close

A retained closed mux previously kept its active inbound stream table and queued pre-handler OPEN/data. Registering a handler later could dispatch that retired table. A complete peer OPEN read before Close could also wait for the stream-table mutex, then create a new entry and dispatch after the terminal signal.

Close now retires each still-active table entry under the stream-table mutex, closes its idempotent local-release signal, returns its queued inbound byte accounting, and detaches the entry. Local Open and read-loop peer admission check terminal state under the same mutex. Handler registration skips the table once the mux has ended. No late admission can repopulate that closed table. Existing receive/release behavior remains terminal and accounting is not returned twice.

## Evidence

Three focused families plus inbound-budget/receive/outbound-close regressions pass 30 fresh local race repetitions. Tests cover a queued unhandled OPEN/data entry, repeated Close and late release/registration, a complete peer OPEN held before admission while Close wins, and 100 concurrent local Open/Close races per test. A source overlay restoring the prior implementation reproduces retained table/budget and peer dispatch after Close. Full uncached repository race tests and vet pass locally. Desktop CI includes the new families at 30 repetitions; collect the exact revision before reporting native desktop results.

The existing post-selection receive test now starts Close asynchronously while holding the accounting mutex, waits for its terminal signal, then releases the mutex and joins Close. This matches the new synchronous accounting cleanup rather than calling Close while intentionally holding a lock Close must acquire. The first broad focused command exposed that fixture deadlock and was stopped; the corrected narrowed test and the fresh repeated/full suites pass.

## Limits

Only table-owned entries are detached. External Stream handles can retain their queued channels/OPEN data until those handles are released by their owners. A peer FIN may already have removed a stream from the active table while its existing consumer owns remaining bytes; that consumer's release still owns its budget. This is not a claim of zero total inbound memory, forced erasure, all-consumer joining or rollback of previously dispatched callbacks. Close waits for its internal stream/accounting locks, not arbitrary user callbacks. Its terminal signal can be observed before all cleanup returns. Wire behavior and active-session limits are unchanged.

No user files or live sessions were involved. No consent/destination/retention policy, Multies mutation, native Android acceptance experiment, signing/account/device operation or release is added.
