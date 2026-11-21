# S24 fixture correction source only

S23 independent once-only inert suite failed: 134 top-level passes, three failures.
Reviewer retained original log; no local rerun or repair-run happened. S23 failure
must remain attached to its revision, not overwritten by later acceptance.

## Source diagnosis

The recorded fixture's decoded link indices are 1..10, including index7
ip6_vti0. This was confirmed by parsing the stored link-*.bin payload records,
not a device read or test run. protocolRig used that captured baseline, whereas
old fakeKernel.Create always creates at index7 and Apply's local-route/link-up
model also hard-codes7. Transaction.Start's first exactHandle check after Create
then matches the preexisting baseline index7 and rejects because that link is
not the newly returned creation handle. Thus Start returns ErrConflict before
alias/readback. The lag test consequently never reached a pending forward step.
This is a source-proven collision; runtime fixes remain untested.

## Correction

Only the new test-owned sessionFake changes. It now selects max existing index+1
from its captured Inventory, creates an AttachCreated handle at that index and
models each exact operation using the supplied canonical handle index. No old
fakeKernel tests or production source changed. The fake models only explicit
address/route state, no claimed kernel-derived automatic route proof. Underlying
value-only close removes the canonical resource and preserves baseline links.

Stronger independent oracles: actual Stop drives a fake close error after modeled
removal; reconcile receives freshly decoded recorded fixture data and rejects an
independently altered MTU. Re-add case first performs one successful delete and
absent readback, then readds identical route while attempts==1/no retry slot, so
ErrConflict must precede budget exhaustion and no fake call occurs. A positive
resolved-poison cleanup record is projected with separately supplied fresh bound
witnesses; historical poisoned state cannot itself supply new authority.

No compile/vet/test execution for this correction. Test source only, review and
one-run release required. No real callback, FD, device, namespace, signal or
socket. Existing live files/Transaction/defaults unchanged.
