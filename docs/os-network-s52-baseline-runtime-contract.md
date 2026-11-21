# S52 pinned baseline comparison and resident runtime contract

SOURCE/fake correction only. No live reads, original open/close, ioctl, helper,
namespace/device/preflight/process/binary execution or author tests/race. S51 input
accepted independently208 inert/12 race/compile, not evidence for S52 behavior.

## Corrected pre-open and post-close comparison

One shared originalBaselineMatches is now used by pre-open source and post-close
pure parser/source. It calls existing exactSetupPostMatches/comparableSetupRaw,
not a new volatile mask or a generic diff. Original evidence bytes remain intact;
comparison copies alone apply the existing pinned v6.17 policy:

- link stats attr7 exactly96 bytes and stats64 attr23 exactly200 bytes, only
  counter data masked with presence/layout unchanged;
- nested IPv6 SNMP attr3 exactly304 bytes/count38 and ICMP attr6 exactly56/count7,
  preserve count/shape and mask only counter tails;
- nested IPv6 cacheinfo attr5 exactly16 bytes, mask ONLY timestamp bytes4..7,
  preserve reassembly/reachable/retransmission fields and raw ordering.

All other raw attributes, safety flags/values, link identity, addresses, routes,
rules, shapes/presence and nesting remain exact. Sysctl maps remain EXACT, including
effective ALL/default operands and safety leaves. No new counter/profile sizes or
policy relaxations were introduced. Unknown/malformed layouts refuse.

The target name must be absent in BOTH frozen baseline and actual raw inventory.
This explicit check refuses even an invalid baseline that already contains the
target. A target TUN/new link still present cannot become absence by comparing
against an equally invalid baseline or ignoring counters. A renamed/replaced/
extra link still differs under whole inventory comparison. Namespace/attempt/
ticket/current owner/close state correlation remains unchanged.

S51 exact raw equality unnecessarily refused ordinary counter/timestamp drift.
This correction reuses already-reviewed narrow policy, making cleanup less brittle
without pretending a generic normalization or operational policy conversion.
Other volatile/unknown differences can still conservatively refuse cleanup.
Sequential collection remains non-atomic and source provenance unexecuted.

## Authored UNRUN tests, actual shared seam

TestOriginalBaselinePinnedCountersOnlyAndTargetAbsence calls the SAME comparator
used by pre-open source, then invokes post-close parser via actual controller.stop
with fake raw supplier. Adds pinned stats64 layout in baseline and actual records,
allows counter-only changes on a foreign/non-target baseline link; safety flag
mutation, malformed counter shape, sysctl safety change, target present and an
invalid target-containing baseline refuse. It asserts original evidence bytes
were not masked in-place, original fake close count1 and PendingCleanup remains
on refusal. These are actual pure comparison/owner parser tests, NOT kernel reads,
source baseline capture or live cleanup proof. Independent tests remain parent-owned.

## Long-lived runtime belongs INSIDE resident work callback

withOperation1ResidentSource is a resident work scope, NOT a function that returns
a working network object for later use. Its callback must contain the WHOLE owned
runtime: eventually reviewed Start, setup/readback, packet supplier lifetime,
peer session/MTU validity, the long-lived packet loops and shutdown/cleanup. The
callback blocks while that runtime is needed, bounded by original owner deadline,
peer session and cancellation. Packet/supplier authority cannot escape scope.

Returning from work means the runtime ENDS. Deferred owner stop attempts the
original nonpersistent TUN close and seeks scoped absence while namespace custody
is still held; then source.active is revoked. A returned Ready object or suppliers
used after work return would be wrong: Close removes the original nonpersistent
TUN and later source operations must refuse inactive scope. No FD duplication,
persistence, raw-number retention or background goroutine escaping scope can
extend it. Resident namespace owner must retain fd/locked-thread custody through
cleanup; helper work scheduled outside that thread is not granted by the mutex.

Packet pumps may need separate runtime goroutines, but ANY source observation/
configuration still requires the resident's bound thread and owner lane. A future
runtime must explicitly route those requests to the resident lifetime owner,
cancel/join packet goroutines before callback return, and never share an active
context lease as ambient authority. No packet runtime implemented here.

Current source still cannot start this runtime: Snapshot returns empty Inventory
and ownedAFPolicyIncomplete, no real launcher/app installs this callback and
missing full-AF creation applicability independently blocks op1 admission. This
is a lifecycle design clarification, not an assertion Start or networking works.

## Smallest next diagnostic-to-operational contract proposal, NOT code

Keep cleanup evidence distinct. Do not change activeInventory to return a private
Inventory merely because close/absence now exists.

Proposed next review unit is phase-scoped setup observation, not broad Ready:

1. Real recorded full-link/AF pass with exact request/reply provenance on resident
   thread and held original FD/netns, covering canonical baseline and owned link.
   It must preserve byte-level attrs and sysctl operands and refuse unknown/missing
   shape, without borrowing inert reader/pass markers.
2. Named policy assessment consumes that pass and complete current ALL/default/
   owned link formulas, returns either a typed phase diagnostic or explicit list
   of still-unsupported gates. No unchecked boolean or general AF26 exception.
3. A separate reviewed operational conversion accepts only a complete successful
   policy assessment plus exact phase transition expectations. For precreation it
   uses the source baseline; after creation/alias it must prove exact original
   down/unaddressed profile; after disable/address/up/route it needs each later
   source-derived raw delta. It can produce setup Inventory ONLY for the proven
   phase, not assert all future steps or active readiness.
4. Transaction/session callers must bind that Inventory to the same owner/attempt/
   generation/Plan and observation sequence, consuming it for one intended setup
   readback. Snapshot's current untyped Inventory return cannot carry that proof
   by itself; the adapter owns the correlation and must fail closed on stale or
   wrong-phase use. No active/display/packet conversion from a phase1 result.

Readiness remains downstream of EVERY step, full IPv6/forwarding/foreign policy,
peer-authenticated MTU/session, packet framing/features/lifetime and installed
host route/firewall/XFRM/overlap checks. The smallest initial implementation would
unblock only precreation/creation/op1 diagnostics; it must still return unsupported
for later phase conversions. Whether existing full-AF word policy actually allows
that narrow operational conversion needs explicit source review before code.

No such conversion added. No ops2-5 or route/address cleanup tokens. Existing
historical activeInventory always-refuse remains. This proposal identifies the
Start blocker rather than solving it with fake canonical Snapshot fixtures.

## Compile-only

Unchanged offline explicit package build with preinstalled verified Go1.26.11,
CGO off, GOTOOLCHAINlocal/GOPROXYoff/GOSUMDBoff/GOFLAGSclear/-modreadonly and only
development+process_launch tags. No tests/run/binary/preflight_launch. Compiler
log delivered; go build does not compile tests or prove runtime behavior. No
source helper release or S19 access retry; no shared sessionproof/Mux/mobile/
session/view changes and no product readiness claim.
