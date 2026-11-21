# S47 operation1-only Kernel source adapter and ownership lane

No actual creation, retained-FD recovery, TUN/helper/namespace/netlink/sysctl/ioctl,
run/binary/preflight or tests executed. Execution remains mechanically unreleased.
S46 independent195 green/14 seam cases/live compile was input evidence only.

## Connected source chain, with honest unreachable preconditions

New operation1Adapter implements Kernel and the same stable session identity
contract used by held resources. Its Apply accepts only the exact Plan operation1
set-alias-mtu-exact. It calls session.begin, then ONE typed source dependency:
operation1OriginalSource.applyOperation1 -> original resources.applyFirstStepSource
-> common orchestrateFirstStepResult -> exact permit/send -> exact ACK -> collect
-> parsePost -> finish. The original source dependency checks the existing
execution guard and namespace-thread/original custody gates. No second Sendto or
generic ApplyExact and no parallel ACK is constructed for the session join.

The actual same firstStepCompletion carries the finished token/ACK/post evidence.
joinFirstStep consumes it once, verifies current exact owner, Plan, operation,
original canonical handle, live deadline, no stop/close/poison and current finished
ledger generation/identity/tag. It records the session ACK, confirms only the
canonical handle tag, advances forwardStep1->2 and clears pending state. It does
not produce an Inventory or call an active conversion. No later operation is
accepted by Apply, including route/address cleanup, which needs its own reviewed
source. Errors leave pending custody and poison forward session progress; a
completion invalidated/canceled/expired between source return and session join
also invalidates packet lineage rather than leaving usable finished evidence.

Removed readFirstLinkPostSource and its unused alternative diagnostic container.
Sender/collector remain dependencies of the one source orchestrator, not alternate
public effect entrypoints. Old pure runInert is fake-only historical coverage.

## Snapshot does NOT bypass the diagnostic Inventory gate

operation1OriginalSource.diagnoseOperation1 locks original file custody, verifies
original device/netns before/after, and returns only a typed ownedSourceDiagnostic.
operation1Adapter.Snapshot validates this diagnostic's owner/session/current
lineage and exact canonical source fields, stores the diagnostic internally, then
returns ownedAFPolicyIncomplete and an EMPTY Inventory. It never extracts the
private inventory field from ownedTUNDiagnosticInventory and never fabricates a
fake AF read pass as source provenance. The canonical diagnostic remains available
internally, not active kernel truth.

This is intentional. Existing ownedTUNDiagnosticInventory.activeInventory always
refuses pending policy. New Kernel.Snapshot has no safe successful conversion yet.
Real Transaction.Start currently cannot pass Snapshot and cannot reach Create or
Apply through this adapter. Create and CloseOriginalFD also explicitly return
ErrExecutorUnreleased; original creation/retention-on-error is a separate gate.
Thus Kernel conformance and internal Apply source connection are NOT real
Transaction.Start integration or product readiness.

newOperation1SourceController creates ONLY private transaction/session bookkeeping
with the source adapter and immutable-backed clock. bindOriginalSource accepts
only resources whose owner/session match that exact controller. Current Create
cannot establish those retained-resource preconditions. Future original creation
must happen within the owner lane, bind the same stable session/canonical handle,
retain custody even on errors and publish no raw FD escape. Constructor/source
method existence is not evidence those facts can currently be satisfied.

Before Snapshot can succeed: each named owned AF IPv4/IPv6 policy field and effective
ALL/default semantics must be complete; real full-AF pass provenance must exist;
strict baseline/raw/address/route/policy transformations and side-effect schemas
must be satisfied; then reviewed diagnostic-to-active conversion must replace the
current always-refuse contract. No extra user option, trusted boolean, fake
supplier or op1 completion can override that gate. Operations2-5, peer-authenticated
MTU/session and installed host path remain independent readiness inputs.

## Serialized ownership, not a fictitious ledger lock convention

operation1Controller owns an unexported transaction/session and mutex. Supported
start, stop and immutable state-copy entrypoints take that mutex for the entire
transaction invocation. Kernel callbacks invoked inside Start/Stop require a
private context lease for THIS controller while the scope remains active. The
lease expires before releasing the mutex. Direct background-context Apply or
Snapshot is rejected before any source call. Session fields, transaction stop/
close/deadline/Ready and clock reads used by the adapter are accessed within this
ownership lane. The source constructor installs a clock backed by private
immutable origin and time.Now/time.Since, no external mutable test clock or setter.

No transaction/session pointer or mutable clock is returned by controller API.
state returns copies, including a copied Handle. ownershipSession is the existing
package-private identity contract needed by source binding, not an external
mutable owner API. The context lease is not passed to untrusted code or installed
as a user callback; live source dependencies are private and synchronous. A future
new API must preserve lane ownership and must not spawn source callbacks using a
lease. Existing independent raw Transaction methods remain intentionally inert
single-owner APIs; this patch does NOT pretend all direct package field writes,
old fake tests or old Transaction.Run were made globally thread-safe.

Lock order on this supported path: controller owner mutex -> original file custody
-> packet ledger as needed. Session joining occurs under owner mutex and a current
ledger comparison; that ledger mutex protects packet generation only, NOT session
writers. Original source releases file mutex after finish and before session join;
a close/config invalidation in this gap is detected against exact finished
identity/generation. Close itself remains unreleased. stop waits for a running
owner scope; this is serialized, not preemptive cancellation or a hard latency
bound. Namespace residency still requires the already-locked resident OS thread,
never inherited from a Go mutex. Every live call keeps its TID/netns check.

## Authored inert vectors, not executed by author

- Actual operation1Adapter Apply with a fake source that calls the SAME common
  orchestrator, permit and raw ACK/readback. Checks one send/read, canonical tag/
  step join, explicit later-op/replay refusal, unleased caller refused and real
  Snapshot gate still returning no Inventory.
- Concurrent fake Apply, controller.stop and controller.state. Source blocks on
  test channels, stop/state use the owner mutex and cannot mutate/read the owner
  until Apply releases it. Intended for independent race-enabled inert review,
  with no real helper tags. No timing sleeps or fake ledger-lock assumptions.
- Source finish-to-join gap: fake close-intent/config invalidation/canceled context/
  monotonic owner expiry. Reject canonical advance and invalidate packet lineage,
  keep Ready=false/cleanup pending; one effect only.
- Transaction.Start control-flow fixture with EXPLICIT fake canonical Inventory
  and fake creation shim. This fixture is not production Snapshot, not real
  original creation and not owned AF policy resolution. It lets Start reach op1,
  consume that one completion, retain tagged handle and then refuse op2 with
  Ready=false and cleanup pending. It cannot count as actual source adapter
  Transaction.Start integration. Production Snapshot never has this fake escape.

No inert tests were run by author. Source compile does not compile test files.
Independent ordinary/race tests, if permitted by main, are the remaining validation.

## Compile records

Source audited for package init/go:generate/go:linkname/C imports: none. Explicit
package build, preinstalled verified Go1.26.11, offline, CGO off, no tests/run/
binary/preflight, no toolchain download. Provisional build passed before adding
finished-completion error invalidation. Final complete production-source build
separately logged. The flags are unchanged:

CGO_ENABLED=0 GOFLAGS= GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
/home/sandbox/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.11.linux-amd64/bin/go
build -mod=readonly -tags 'mnet_enrollment_development mnet_os_process_launch'
./experiments/osownership

No fresh external kernel claim added. S46's pinned later2-5 design remains source
analysis, not implementation. No step2 mutation/reader, retention-on-error source,
shared Mux/sessionproof/mobile/session/view changes or installed host proof here.
The source adapter and completion join reduce a specific wiring gap while keeping
all unresolved policy, creation, effect and user-deployment gates visible.
