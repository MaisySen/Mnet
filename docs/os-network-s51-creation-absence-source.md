# S51 adapter-owned creation and resident-scoped postclose absence SOURCE

Scope described to parent before implementation and accepted for source/fake seams.
No actual open, close, ioctl, device/netlink/sysctl read, helper, preflight, binary
or process run. No author tests/race run. Operational Snapshot remains fail-closed.
S50 independent206 inert/10 race/final compile was input evidence only.

## Intended connected caller flow

One intended resident work entry withOperation1ResidentSource creates the private
controller/adapter and installs ONE resident-scoped source as both creation and
cleanup-absence supplier. It is intended for residentBootstrapSource's existing
locked-thread work callback, NOT installed in a launcher/app or released to run.
Its scope checks current namespace custody before entry. It finalizes owner stop
before marking source inactive and returning to the resident namespace owner.

Future flow is resident work -> controller.start -> Transaction.Start ->
operation1Adapter.Create -> published originalAttempt -> resident source
createOriginal -> createTUNSource/openExclusiveNonpersistentTUN -> exact pure
bindOriginalCustody -> session.bindCreated -> resident original-resource retention
and actual original-device verifier -> diagnostic source -> op1 Apply chain.
There is no second original open or detached controller creation helper. Removed
unused separate createOriginalAttemptSource/bindOriginalSource entrypoints.

IMPORTANT: real Transaction.Start still stops at Snapshot, which returns no active
Inventory. This source connection cannot make actual Start reach creation. The
creation source is now reachable through the intended Kernel.Create dispatch
rather than a standalone gate, but full operational Snapshot policy/provenance,
app/resident wiring and execution release remain absent. Fake Create join tests
are NOT real Start integration or product progress proof. No Ready path added.

## Creation ownership/error guarantees

Adapter.Create requires its private active controller owner lease. It publishes
one copied-Plan attempt before calling its creation dependency, marks the session
create-attempted and pending custody, refuses a second attempt and never retries.
Source confirms the same immutable baseline raw/sys is still present BEFORE open.
That comparison is complete exact equality, not an assumption based on names.

The S49/50 opener retains the original file before fallible checks. Any postopen
failure returns error with the same controller attempt/custody, even no Handle.
A pre-open read/open failure has no original custody and owes no original close.
The source driver retains its ownedCreation result even on error. Adapter returns
any available Handle WITH the error, marks session poisoned and leaves controller
attempt obligation intact. An available Handle does not turn failure into success.

Success requires same retained custody and optional validated Handle matching the
returned Handle. Session.bindCreated validates the exact Plan/Handle. Source then
retains the same canonical session/namespace/original file/ledger and checks the
kernel-derived original device/netns/name before returning a diagnostic supplier.
The initial retained identity is correlation, not a claim those kernel checks ran.
A failing actual verifier returns error and preserves original obligation. The
source remains guarded/UNEXECUTED. Full creation applicability/full-AF policy gate
is still not implemented, so op1 begin can independently refuse missing AF
creation binding. No fake source read pass is manufactured to get around it.

Owner stop and failed controller.start continue the same original-close path:
stop/Ready revocation, exactly-once original close regardless Snapshot/Handle,
then bounded absence if available. No policy error, failed session bind or
ambiguous creation can make the controller forget its original attempt. Source
callback finalization also calls stop on error/cancel before ending its scope.
No route/address cleanup tokens or ops2-5 are enabled.

## Exact postclose absence, separate from operational Inventory

operation1ResidentSource.observeOriginalAbsence is SOURCE only. It requires this
controller's current attempt, same retained namespace and baseline pointer, current
namespace FD/TID check, a close-attempted original whose fileCustody.closed is set,
and a currently active per-stop ticket. Under a500ms context cap after close, it
collects ReadStableInventory then readPolicySysctlsSource from the SAME held
namespace, checks namespace/lifetime/current ticket again, and runs the shared
pure parseOriginalAbsence equality/correlation check.

The pure check requires exact complete raw inventory and sysctl map equality to
this owner's frozen precreation baseline, namespace binding, same attempt/owner,
original custody, close-attempted state and current ticket/sequence. The source
reader supplies kernel sender/request/reply framing through existing raw readers;
no fake AF reader or operational Inventory conversion participates. Unknown attrs,
extra/missing state, raw/schema differences and sysctl extras are differences,
not silently ignored. No new volatile mask was introduced.

That equality is CONSERVATIVE and can refuse cleanup on counter/cache/timestamp
drift, even when the owned link is actually gone. It is not an operationally
reliable cleanup service yet. It also depends on the source baseline candidate
and complete supported raw/sys readers; source code does not establish they were
captured or executed successfully. Sequential reads plus before/after namespace
checks are not atomic proof against every transient uncontrolled kernel change.
500ms is an approximate context bound, not a hard syscall/scheduler deadline.

## Ticket consumption, lifetime and sticky errors

Each stop gets a fresh increasing sequence and private ticket bound to exact
owner/attempt. Ticket active only during that stop's observation. Evidence must
carry the exact same ticket pointer AND original custody/attempt; stop rechecks
current sequence, owner, active/unused and live context before consuming. Ticket
is marked used and deactivated/removed before return on every path. Even a prior
successful absence result cannot be replayed on another stop. Every stop that
seeks clean state makes a new observation, not reuses cached equality.

A successful close is still not absence. Complete equal raw evidence may confirm
observed absence even after close failure, but sticky original close error keeps
PendingCleanup=true. Clean absence cannot erase failed close or authorize another
close. Valid absence plus successful original close clears ONLY PendingCleanup,
never Ready/session active authority. Failed/partial/wrong namespace/stale/replayed
proofs retain pending cleanup. Original closer stays once across repeated stops.

Namespace ownership remains with resident's existing namespaceCustody, not this
adapter. Source must not escape its work lifetime: scope.active becomes false on
exit and every source call checks it plus namespaceCustody FD/TID/current identity.
Diagnostic supplier is wrapped with that same scope check. Escaped controller/source
calls after scope exit fail before reads/open. Ticket cannot survive stop return,
and cannot mint outside the resident scope. No namespace descriptor is closed or
reopened by this adapter. A failed namespace read/check blocks absence, not the
owed original Close already performed. Source scope is not a grant to run on a
different Go goroutine/thread; resident locked-thread precondition still applies.

## Authored UNRUN fake validation and exact limits

- Actual adapter.Create dispatch with fake creation dependency checks attempt
  already published; open-failure/no custody, shared afterOpen injected failure,
  no-handle retained error, successful canonical bind, failing source-bind callback.
  Stop twice must close fake original once or zero if no open, never Ready.
  These are adapter/dependency join tests, not live opener/kernel verification.
- Shared pure parseOriginalAbsence with baseline exact records, raw drift, sysctl
  extras, namespace mismatch and failing closer. Equal records can clear only
  pending cleanup when close succeeded; failure remains sticky.
- Captured successful proof/ticket becomes inactive+used after stop and is refused
  when fake supplier returns it for a later stop. No close retry.
- Existing fake absence supplier was updated to carry the current stop ticket;
  previous wrong-attempt/close-order/error/stop-lane tests are preserved.

No fake test claims production reader provenance or actual resident lifetime
execution. Live source guards, ns checks, baseline collection and wrapping are
source-read+compile only. Independent ordinary/race review remains parent-owned.

## Compile-only validation

Hook audit carried forward, no new init/generate/linkname/C imports. Provisional
and final production-source explicit package builds succeed with preinstalled
verified Go1.26.11, offline, CGO off:

CGO_ENABLED=0 GOFLAGS= GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
/home/sandbox/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.11.linux-amd64/bin/go
build -mod=readonly -tags 'mnet_enrollment_development mnet_os_process_launch'
./experiments/osownership

No tests/run/binary/preflight_launch/device/helper/access workaround. go build does
not compile test files or prove runtime lifecycle. No full Inventory policy,
ops2-5, route/address cleanup, peer-authenticated MTU, installed host app path,
shared Mux/sessionproof/mobile/session/view edits or Ready claims.
