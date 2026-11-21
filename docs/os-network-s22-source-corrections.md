# S22 source correction gate, not runtime evidence

No compile, tests, mutants, device/system effects or activation were performed.
The previous S21 archive was a full tracked tree without a root checksum file.
It must not be described as having passed a root manifest check. This packet
adds a root SHA256SUMS and PACKET-LAYOUT.md outside the tracked source tree.

## Corrections to S21's table

- Cleanup choice validates completely before changing cleanupMode. Refused choice
  or close does not itself disable setup. An already poisoned setup stays poisoned.
- The first admitted delete permanently enters cleanup. Per exact Operation the
  session has TWO total delete attempts, never two retries. After an attempt the
  next fresh exact Rollback readback records present/absent. If present, pending
  clears into an explicit identical retry slot; Transaction's own remaining-op
  check returns cleanup pending. A later Stop may retry that identical operation
  only after another fresh exact Rollback selection. After attempt two there is
  no further delete. No changed target, broader delete, or close retry is allowed.
- Call error/ACK and readback presence are recorded separately in bounded cleanup
  history. Unknown/conflicting readback leaves pending. No early readback grants
  a retry. An item that disappeared can proceed to the next exact cleanup step.
- JSON copying was removed from the new adapter session. Structural cloning
  preserves slice/map nil versus empty, attribute byte slices, rules ordering,
  scalar netip values and zones. Existing earlier baseline-capture implementation
  remains unchanged; this corrects additional copying in this new ledger only.
- An invalid returned creation handle is retained as rejected custody. Subsequent
  fake Snapshot returns unresolved-custody error, so Transaction.Stop cannot
  observe an empty inventory and declare done. No handle adoption or implicit
  replacement close is attempted. Unknown creation without valid custody also
  blocks all subsequent protocol snapshots.
- Failed/lagging forward readback immediately poisons setup, keeps pending as
  possibly applied and never advances. There is no wait-until-success promotion.
- Close's fresh Snapshot reuses only the still-live, deadline-bearing owner
  context from the preceding Transaction snapshot. It gets a 100ms child cap,
  never Background. A canceled/expired/no-deadline context refuses. This models
  bounded ownership through the old no-context CloseOriginalFD interface; the
  fake method itself is cooperative and cannot certify a hard syscall timeout.
  No live implementation is allowed under this interface experiment.

## Capability evidence boundary

Naked verified/live booleans were removed. Deployment, OS path, transport and
original authority each require a witness bound to identical nonempty target,
session, isolated namespace and a nonempty observation/expiry interval containing
the supplied trusted Now. Expiry of the projection is their earliest ValidUntil.
Duplicate valid witnesses of a kind refuse readiness as ambiguous. Wrong target,
future observations and expired witnesses cannot create readiness. Readiness is
only the specified path in this projection, never proof of all app networking.
These witnesses are inputs, not independently authenticated by this pure helper;
no production caller or witness supplier exists. Wall-clock distrust must make
the caller refuse projection; Transaction still owns actual monotonic/absolute
authority and no helper extends it. Runner denial stays a separate observation,
user deployment remains unknown without a bound deployment witness, and no
projection initiates an action.

## Additional proposed test matrix, not executed

Exact cloning: nil/empty outer slices; nil/empty rules and rule byte arrays;
nil/empty attribute maps and values; invalid/IPv4/IPv6/zoned netip fields; nested
mutation isolation in both directions; original baseline equality after cloning.

Cleanup: one failed delete with unchanged exact readback then successful second;
two failures exhaust; failed call that actually removed item; readback transport
error keeps pending; replaced link/policy/namespace refuses; changed operation
cannot consume retry; refused cleanup does not change phase; no delete after
close; failed close not retried; baseline-equal failed close still unresolved.

Context/custody: no-deadline/canceled/expired preceding context; cooperative close
read timeout; rejected handle plus empty snapshot never completes Stop; no-handle
creation ambiguity; snapshots cannot adopt custody. No real processes or syscalls.

Capability: target/session/namespace mismatch, future/expired evidence, duplicate
kind, missing authority, cleanup pending, transport only, runner denial versus
unknown deployment, exact earliest projection expiry. A source gate precedes all
these tests. Existing accepted tests and live files remain unmodified.
