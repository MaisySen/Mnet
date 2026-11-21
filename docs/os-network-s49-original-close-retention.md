# S49 original custody retained on failure and close owed before observation

SOURCE and fake-only implementation. No actual open/close/device/ioctl/read/helper,
preflight, process or binary executed by author. No tests/race run by author.
Offline compile-only validation is reported separately, not kernel behavior.
S48 inert199/race4/live compile is input evidence, not S49 validation.

## A. Owner close is independent of Snapshot, Rollback and Handle

operation1Controller.stop no longer calls the generic Transaction.Stop loop. It
acquires the exclusive owner lane even when caller context is already canceled.
stopOwnedLocked revokes Ready and marks stopped, validates this controller owns
the attempt, then closes the recorded original BEFORE any optional observation.
A missing/foreign Handle, a failing Snapshot, untrusted Rollback input, missing AF
policy, canceled context or expired setup deadline cannot early-return past the
recorded original close. No route/address deletion is attempted by this workflow.
A legacy handle with no attempt record is a named conflict, not permission to
adopt/close by lookup. A valid published original is required to identify a close
target; absence of that record cannot safely be repaired from a raw fd/name.

start also finalizes a retained attempt on every returned Start error, including
early canceled context before the forward owner scope starts. Finalization takes
the same owner mutex independently of that canceled context. A stop queued while
Apply holds the lane waits for the entire in-flight operation, then attempts the
original close. No concurrent Close overlaps its source descriptor callback.
This is serialization, not hard preemption: a stuck owned syscall can still delay
close, requiring the separately held process-supervisor policy. No source mutex
is claimed to make kernel stalls interruptible.

originalAttempt.closeOriginal consumes close-attempted exactly once and preserves
the result. fileCustody retains its own once guard and sticky error, invalidates
packet lineage before the closer effect, and closes the ORIGINAL object. A private
originalCloser interface enables fake count/error/order checks. Production
constructors set the closer to the exact same original *os.File stored for source
access. There is no raw-fd close retry, reopen, duplicate, path/name adoption or
DELLINK. The common fileCustody code moved from custody_linux.go to file_custody.go
because its lifetime/once semantics have no Linux syscalls and are shared by the
common owner-lane/fake workflow; Linux original verification still needs *os.File.

## Close versus absence versus PendingCleanup

After close attempt, only a separate private typed originalAbsenceSource may
return absence evidence bound to this attempt and the same original custody.
That capability is NOT Inventory and cannot set Ready or permit setup. Production
has NO absence supplier or mint until complete raw reconciliation/provenance is
reviewed. Only tests supply explicit fake absence. Therefore production close
success remains stopped/cleanup-unverified with PendingCleanup=true, not clean.

Failed/partial/wrong-attempt absence is refused. A successful original close does
not prove raw network absence. A clean absence response does not erase a sticky
close error; PendingCleanup remains true. Repeated stop can seek absent-state
confirmation if still unverified but can NEVER call the original closer again.
After verified absence and successful close, fake reconciliation may clear only
PendingCleanup, never Ready. Original creation and close errors remain stored in
the attempt/transaction record; historical errors are not rewritten as success.
No readiness-policy waiver or partial Inventory is used to enable teardown.

The new cleanup flow never enters Snapshot or Rollback, so their availability
cannot skip Close. Exact route/address cleanup tokens and a production absence
reader are deferred, not silently implemented as a fake supplier. Existing generic
Transaction.Stop remains inert/unwired for this source adapter; supported source
owner entrypoint is controller.stop. Kernel.CloseOriginalFD still refuses and no
app/bootstrap installs this partial adapter.

## B. Immediate attempt/file custody before fallible post-open checks

createOriginalAttemptSource is guarded, unused SOURCE inside the ownership lane.
It publishes originalAttempt(owner,Plan,unopened) BEFORE calling the opener and
marks cleanup pending. The opener requires that exact controller-published empty
attempt. It retains the resident namespace/TID preconditions and unchanged
kernelExecutionReleased guard. No source release or access workaround is added.

After successful unix.Open of the exact TUN node, the first application operation
wraps the valid descriptor into *os.File. The same file is immediately installed
in fileCustody; shared afterOpen publishes that custody to the attempt BEFORE
invoking any fallible F_GETFD/ifreq/current-namespace/TUNSETIFF/name check. The
old failure defer that ignored unix.Close(raw-fd) result is removed from this
ORIGINAL TUN opener. Subsequent errors return file WITH error and leave its
attempt/custody intact, including ambiguous TUNSETIFF or kernel-name mismatch.
No other namespace/route/socket descriptor's separate lifetime was changed here.

The shared afterOpen seam is used by both source checks and fake retention tests.
Its empty-attempt precondition is checked before open under exclusive ownership;
there is no intervening external callback that can replace it before publication.
It must not be reused as a generic raw-fd adoption API. A future new caller must
retain this sequencing and cannot open first then attempt to find an owner.

bindOriginalCustody validates raw link/AttachCreated while retaining the same
custody on every error. ownedCreation can now return with error and no validated
Handle, via its originalAttempt.handle=nil, instead of returning nil and losing
an uncertain close obligation. Its Handle value is not valid authority until
attempt.handle is present; existing session/resource checks still refuse zero or
mismatched Handle. On success the exact Handle is copied into the attempt and
stage advances. No alias/MTU/up/address/route claim is made by creation.

The older bindOriginalFD value-binding seam now also returns retained custody on
failure, never hides a close attempt. Its caller owns the returned record; live
creation uses controller-published attempt rather than this legacy convenience.
Its inert test changed from expecting automatic untracked close to asserting
error-with-custody/no handle and then one owed fake close. Existing successful
value-binding checks remain unchanged.

createTUNSource preserves a nonnil ownedCreation with custody on post-open error.
The controller independently holds the original attempt even if no creation
result/Handle can be returned. bindOriginalSource now additionally requires the
attempt/custody/validated handle belong to that exact controller's owner/session;
no unrelated retained original can be installed.

## C. Decisive fake seams and limits

Authored, UNRUN:

- private closer records exact count and error, checks three repeated closes
  invoke it once with sticky error;
- closer checks packet lineage is already closed/uncertain BEFORE its fake effect,
  including a failing close, then verifies no retry;
- no-handle Stop without observer; failed/wrong absence; clean fake absence;
  failed close WITH clean fake absence; canceled no-handle Stop; actually failing
  fake Snapshot followed by Stop without another Snapshot; foreign handle that
  would be unsafe Rollback input cannot redirect or block original Close;
- order log requires original close BEFORE absence. Missing/failed absence keeps
  PendingCleanup; clean absence cannot erase failed close. Repeat Stop never
  repeats the closer;
- shared afterOpen fake checks run only after attempt.custody is installed; modeled
  post-open/CLOEXEC, ambiguous ioctl, readback, handle-validation and session-bind
  failures retain the same obligation, optional Handle and error. The fake stages
  are sequencing probes, not kernel syscall behavior or complete creation coverage;
- actual bindOriginalCustody pure readback failure retains nonnil record/custody,
  no validated Handle/no close until owed finalization;
- canceled controller.start still finalizes the recorded original;
- Stop queued during fake Apply: pre-call arrival marker and bounded30ms channel
  assertion that closer has NOT run before Apply is released; after both goroutines
  join, order must be effect-complete then close, count1, Ready=false and pending
  absence. No racing count reads. S48 bounded lane oracle also remains.

No test calls a real opener/device/helper or uses an actual os.File closer for
these new decisive assertions. Existing older temp-file tests still exist but
were not executed by author. Parent controls any independent inert/race run.
Source go build does not compile test files.

## Validation and remaining gates

Same explicit offline package compile permission only: preinstalled verified
Go1.26.11; CGO_ENABLED=0 GOFLAGS= GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off;
build -mod=readonly -tags 'mnet_enrollment_development mnet_os_process_launch'
./experiments/osownership. No preflight_launch/go test/go run/binary/helper. First
provisional compile log retained; final source compile separately logged after
shared afterOpen and owner-finalization changes. Source hook audit found no
package init/go:generate/go:linkname/C import.

Operation1 source remains unreachable through real Transaction.Start: operational
Snapshot still always refuses, Create remains unreleased and bootstrap/app wiring
is absent. This slice fixes the original obligation path in source and the owner
cleanup design; it does NOT establish live creation, live close, working network
or usable product behavior. No ops2-5, exact route/address cleanup tokens, full
operational Inventory, real absence supplier, authenticated peer MTU, installed
host route/firewall/XFRM/CGNAT path or Ready code was added. Existing S19 EACCES
not retried. No shared Mux/sessionproof/mobile/session/view edits.
