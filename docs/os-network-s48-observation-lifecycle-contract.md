# S48 proposed observation and creation contract, NOT implementation

Review gate only. No operations2-5, readiness, creation, cleanup adapter wiring,
reads, helpers, TUN/device retry or kernel execution. S47 remains unwireable:
Snapshot always errors, therefore existing Transaction.Stop returns before original
Close. Its Kernel conformance is not a usable lifecycle. Do not install it.

This design separates three different capabilities. No successful conversion is
implemented in this slice. Existing activeInventory always-refuse gate stays.

## Minimal contract split

| Evidence/result | Permits | Must never imply |
| --- | --- | --- |
| Diagnostic observation | Inspect retained raw facts and explicit refusals | Setup success, destructive lookup, packet permission, Ready |
| Cleanup observation | One exact teardown choice or explicit refusal, bound to original custody and current attempt | Application networking safety, forward setup, replacement deletion, active Inventory |
| Operational observation | Existing strict full-policy Inventory for setup/session transitions | Ready without all-step ACK/readbacks and remaining authority gates |

Proposed cleanup observation is a private, immutable, single-use scoped value,
NOT Inventory and not a caller-supplied boolean. It carries held namespace and
attempt identity, original object reference, current generation, collection
sequence/time/expiry, raw source provenance, original-fd close state, and complete
facts relevant to the ONE proposed action. Missing/contradictory fields are named
refusals. Raw source data cannot manufacture an action token by itself.

Proposed operational observation has separate conversion that revalidates every
existing full-AF/effective/forwarding/route/policy gate. A cleanup value has no
method to produce this type or Inventory. Ready remains exclusively downstream
of complete original creation and all-step session transitions, live owner,
peer-authenticated session/MTU, packet capability and installed host path. No
readiness flag or generic ignore-policy mode is added to help teardown.

## Why existing Kernel Snapshot is insufficient

Kernel.Snapshot currently serves Start, Apply readback, Rollback and Stop. An
Inventory accepted for cleanup is indistinguishable from one accepted for setup.
S47 correctly refuses diagnostic promotion, but then Stop returns at its first
Snapshot error and skips original Close. Returning a partial Inventory would
silently weaken setup/Ready. Reinterpreting an error as empty inventory would
silently destroy information and is also forbidden.

Proposed new private lifecycle contract: owner controller invokes a distinct
cleanup workflow over typed cleanup observations, not Transaction.Stop's current
Snapshot loop. Existing generic Stop stays unwired for this source adapter until
that workflow is reviewed and implemented. The workflow must preserve failure/
cleanup evidence back into the transaction without granting setup authority.
No new workflow or special-case error swallowing is implemented in S48.

## Original close cannot depend on readiness or successful network inspection

Once an attempt owns an original nonpersistent file, the owner owes exactly one
close attempt even if diagnostic/policy/namespace readback fails, the context or
owner deadline ends, setup never started, or no safe link identity was established.
Closing the ORIGINAL retained os.File is not deleting an interface found by name.
The original file is the close target; no raw-number recovery, lookup/adoption,
DELLINK, persistent-link conversion, dup/export or external revoke is allowed.

Proposed teardown ordering:

1. Enter exclusive owner lane, mark stopped and Ready=false, revoke packet
   authority and cancel forward work BEFORE any further source action. A fresh
   cleanup deadline is a bound on cleanup work, not renewal of setup authority.
2. Try a bounded cleanup observation for recorded exact owned side effects when
   needed. Inability to observe blocks route/address destructive effects, NOT
   the owed original close attempt. No snapshot error may early-return past it.
3. If exact route/address cleanup is authorized by a current cleanup observation,
   remove only that exact owned item through one-use scoped effect tokens. Failure
   or ambiguity remains recorded. This optional step cannot stall close forever;
   failed/canceled reads or deadline exhaustion skip these effects and proceed.
4. In an unconditional close-finalization path, invalidate lineage before actual
   original Close, consume original-close once state, call that os.File.Close
   exactly once and retain the error permanently. No second close or reopening a
   path/raw fd if it fails. Original close does not need TUN ioctls or a new
   namespace lookup. Must not run while another owned source call holds file
   custody; owner-lane cancellation/supervisor is required for stuck calls.
5. Only after close attempt, collect bounded absence evidence if possible, while
   retaining namespace custody. Verified clean completion needs successful close
   AND raw baseline/owned-state absence at a fresh checked collection. Close
   success alone is not absence or readiness. Absence alone cannot clear a sticky
   close failure. If source inspection remains unavailable, report stopped but
   cleanup-unverified, keeping pending custody/errors and no retry permission.

Linux nonpersistent TUN teardown may remove connected kernel state on final close,
but this design does not assume every recorded owned route disappears or that a
partially observed foreign replacement can be touched. Exact post-close raw
reconciliation must establish what remains. No ordinary read denial is permission
to workaround access or take a stronger deletion action.

## Cleanup-specific safety facts

For original close: exact retained *fileCustody ownership/once state and exclusive
lifetime reference suffice to identify the target, even without ifindex/name.
Open network namespace or device identity still matters to observations, but
failure of those checks cannot change the original close target.

For route delete: one current full all-table route/rule observation with exact
owned destination/output ifindex/protocol/table/scope/type/metric, baseline/attempt
provenance, current original-link identity and no replacement/conflict. Source
normalization/duplicate semantics and one-use request binding must be reviewed.
No policy unknown is ignored if it can change route ownership/interpretation.

For address delete: current exact address representation, scope/flags, original
ifindex/object/netns identity and attempt record, no foreign replacement. Derived
local routes are observed as kernel-derived effects, not freely deletable owned
protocol242 routes. Unknown flags or unexpected sibling state refuse effects.

For absence: complete raw baseline comparison with explicit supported volatile
fields and all relevant routes/addresses/links/rules, current namespace custody,
collection identity/sequence, successful close state. Partial dump, error, stale
sequence, namespace switch or missing provenance cannot certify absence.

Cleanup need not prove peer MTU or end-to-end packet admission, since it never
permits packets. Effective AF/ALL/forwarding values not needed to identify a
particular original Close are not a close prerequisite. For destructive network
effects, include each policy fact that bears on ownership/scope/normalization,
not all readiness policy by reflex and not an unreviewed omit-all shortcut. The
specific minimal schemas for cleanup route/address effects still need review.

## Full diagnostic-to-operational policy gates remain unchanged

Real full-AF reader provenance must bind actual request/reply sender/seq/recipient,
namespace/thread, ifindex/name, raw nested schema/flags and canonical sysctl reads.
No inertAFReader or constructed pass substitutes. Original FD identity and
TUNGETDEVNETNS correlation bracket the collection; status flags/framing/names/
ACTIVE feature compatibility are actual source observations, not labels.

Every named IPv4/IPv6 DEVCONF word and effective ALL/default formula must have
reviewed semantics and current complete operands. Baseline immutability and exact
owned raw deltas apply across links, rules, addresses and all route tables.
Forwarding, redirects, source validation and foreign policy remain explicit.
Phase-dependent disabled-IPv6 state includes addresses/routes/DAD/related raw
side effects, not one leaf. Missing IPv6 tree is unsupported unless independently
proved inapplicable. No host-global sysctl hardening is proposed.

An operational conversion may produce Inventory only when these policies are
implemented and checked. Sequential collection is bounded and correlated, not an
atomic guarantee against uncontrolled kernel writers. All-step transition tokens,
current canonical handle and owner deadline must still match. These requirements
are not implemented by this design and activeInventory still always refuses.

## Creation and error retention inside the owner lane

Current source gap grounded in repository:

- kernel_linux.go openExclusiveNonpersistentTUN uses a raw fd until successful
  TUNSETIFF/name verification, then wraps as os.File. Failure defer unix.Close
  does not retain its close error. A post-ioctl name mismatch can therefore return
  no file and lose a failed-close custody obligation.
- create_custody_linux.go bindOriginalFD wraps a file in fileCustody, observes a
  link and returns ownedCreation only when exact AttachCreated matches. On
  observe failure it closes before returning nil, joining close error but not
  retaining a typed object through which the controller can track ambiguity.
- ownedCreation requires a validated Handle. It cannot represent an opened or
  ambiguous successful-creation fd before link identity is known.
- retainOriginalPacketResources additionally requires a canonical phase1
  session, which is not available on those error paths.

Proposed minimal attempt record, owned by controller before first open: attempt
identity/Plan/ns custody, stage enum, original *fileCustody optional, validated
Handle optional, create error, close-attempt/error fields and reconciliation
status. Stage distinctions: unopened; original-open-no-link-proof; ioctl-attempted
ambiguous; original-link-validated; session-bound; stopped/close-attempted; reconciled.
The record must permit an error WITH original custody and WITHOUT a Handle.
No field may suggest that a name/ifindex observation alone adopted the original.

Proposed sequence under the SAME owner lane and resident locked thread:

1. Validate plan/authority/deadline and capture a complete pre-creation baseline.
   Publish the attempt obligation internally before any effect.
2. Open exact TUN device once, immediately wrap original descriptor in fileCustody
   and store it in the attempt, before any later failing check or ioctl. Raw number
   stays within controlled lifetime and is not reconstructed after error.
3. TUNSETIFF exclusive/nonpersistent exact name, inside supported descriptor
   access, with create-intent recorded before it. Errors are ambiguous stage,
   not permission to retry or assume no interface. Keep file and namespace custody.
4. Correlate original kernel device/netns/name and exact raw link, construct Handle
   only after proof, bind the SAME session, ledger and resources under owner lane.
   Any failure keeps the attempt and original file even without a valid Handle.
5. Return a result containing attempt custody and error. Kernel.Create's current
   Handle-only return cannot express all of this, so controller must own attempt
   before calling it; no no-handle error loses the record. Original close is owed
   from the attempt, not contingent on Transaction.handle being nonnil.
6. On any failure/cancellation, enter cleanup finalization above. Close exactly
   once, retain sticky failure and bounded absence reconciliation. No automatic
   retry, node creation, permission changes, inherited-fd access or S19 workaround.

Required future fake matrix before a source release: failure before/after open,
CLOEXEC check, TUNSETIFF ambiguous result/name mismatch, original ioctl/netns check,
raw readback, AttachCreated/session bind/ledger attach; close success/failure;
concurrent stop; timeout at each boundary; no-handle failed creation with retained
obligation; borrowed/reused fd rejection; successful close but failed absence;
raw absence but failed close. No real creation implementation or tests added now.

## S48 actual delta and validation scope

Removed unused cached diagnostic state from operation1Adapter; Snapshot validates
then returns its held error, with no retained dead value or success path.

Strengthened lane oracle: Stop/state goroutines mark arrival immediately before
attempting their controller calls. Test waits bounded1s for BOTH arrival markers,
then requires neither returns during a bounded30ms while fake Apply still owns
the lane. Only after that negative assertion is the blocked fake source released;
then all goroutines complete and stopped/cleanup/step results are checked. This
shows observable blocked-before-release behavior, stronger than a race pass alone.
A pre-call marker is not instruction-level proof that the goroutine ran the lock
instruction, so this is a bounded concurrency oracle plus source lock review,
not a universal scheduler proof. Author did not run tests or the race detector.

Compile-only permitted offline package check uses preinstalled Go1.26.11, unchanged
explicit package/development+process_launch tags, CGO off, no test/run/binary or
preflight_launch. It does not compile tests and proves no lifecycle behavior.
No readiness or cleanup conversion/code, new creation or operations2-5 here.
