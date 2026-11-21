# S1 source transaction and fake-kernel evidence, not kernel execution

This is the next SOURCE packet after S0b1d40f4. No namespace, capability, TUN,
netlink socket, chmod, sudo, install, host fallback or crash signal occurred.
No real Kernel implementation or entrypoint is wired. Normal product/default
code and the prior pair evidence remain untouched. Runtime policy is still a
proposal and makes no human/service grant.

## Changed boundary

1. Plan now requires a disable-owned-link-IPv6 step BEFORE address/up, with
   acknowledged per-link readback. Route includes type unicast, metric and
   priority42700, table254, scope link, protocol242 and exact peer/32/ifindex.
   MTU1280, own/32, no prefix route. Kernel-created own/32 local route is an
   explicit separate entry, not our peer route.
2. Inventory explicitly represents kernel-derived IPv6 address/route entries.
   Chosen policy is per-interface IPv6 disable, not netns/host global sysctl.
   Unexpected IPv6 after up causes start refusal, never ready. Cleanup does NOT
   fail simply because the exact owner acquired verified kernel-origin
   fe80::/64 or ff00::/8 state. Those exact link-local/multicast entries are
   recorded, NOT individually deleted, then the original nonpersistent TUN FD
   closes. Foreign origin, wrong link, wrong route type/proto/table/metric or
   foreign address is conflict and remains pending, never swept away.
   KernelOrigin bool remains model DATA. A real netlink decoder must establish
   protocol/flags/address origin and exclusive custody; none exists yet.
3. Transaction records intent then ACK+readback for EACH step, retains original
   custody and pending cleanup on every partial failure, including timeout after
   success. An ACK without effect fails. Stop reads back before every exact
   deletion, compares full resource identity, readbacks after each deletion and
   handles timeout-confirmed-absence without assuming timeout means no mutation.
   A replaced ifindex/name/alias/type/metric/priority/namespace causes no cleanup
   mutation. The alias is a writable tag, not a secret or sufficient authority;
   exclusive supervisor-owned namespace remains a required security boundary.
4. Owner captures min(self,peer) once with wall and monotonic elapsed bounds.
   Backward wall-clock motion cannot extend monotonic deadline. Start operations
   receive an owner-bounded context; readiness is withheld after deadline. Run
   owns its own time.Timer and calls bounded cleanup on deadline or context end,
   not requiring caller Stop. Cleanup has separate500ms observation context,
   not extra network authority. Backend context compliance and join safety are
   still requirements, not proven driver behavior. A hung backend may require
   the separately reviewed supervisor; no goroutine is silently abandoned.
5. Strict finite netlink framing client runs only through fake in-memory wire.
   It checks kernel sockaddr PID, destination/sequence, capped lengths,
   truncation, alignment, multipart completion, interrupted dump, negative ACK
   and embedded original request. Supports exclusive CREATE and DUMP framing
   only. No socket transport, attribute/rule/sysctl decoder, operation encoding
   or live delete codec is present. It cannot configure any kernel.
6. Linux nsfs/TUN functions are source-only under constant unreleased guard;
   they return BEFORE ANY syscall. No tests call them. Source duplicates FD
   CLOEXEC, checks nsfs, NS_GET_NSTYPE, current-target identity and different
   original parent namespace, retains namespace custody FD. TUN open uses
   CLOEXEC|NONBLOCK and IFF_TUN_EXCL, no persistence, verifies FD_CLOEXEC and actual
   name, closes FD on all failure branches. It never changes namespace.
   Namespace ancestry/userns ownership/exclusivity and parent-death supervisor
   are still missing prerequisites. The guarded functions are not a release or
   complete proof: an FD comparison alone cannot prove isolated safe ancestry.

## Deterministic fake evidence

Old pure S0 tests retained with five-step plan updates. New fake-kernel tests
cover all five step timeout-after-success cuts, ACK without effect, per-step
records, partial cleanup, failed delete pending vs confirmed absence, monotonic
expiry despite wall rewind, ifindex/alias/metric/priority/type replacement and
foreign IPv6 refusal. Kernel-added link-local after fake up fails readiness but
closes original owned link successfully without a broad IPv6 delete. Owner Run
uses a short timer and joins fake resources without an explicit caller Stop.
No real packet or OS rollback follows from these unit names/results.

## Still not ready for live S1

Before any execution request, complete and review kernel transport/strict
netlink attribute decoder, exclusive RTM address/link/route operations and
per-ACK kernel inventory, per-interface IPv6 disable readback, rule/forwarding
inspection, current namespace check immediately before mutations and resource
custody through partial create. The actual nonpersistent-FD cleanup and kernel
side effects must be observed, not inferred from this fake. The pure model does
not authenticate user/session/grant data and must not be plugged into production.

S1 proposed packet test remains actual ordinary TCP SYN via real TUN in a fresh
contained namespace, source address own/32, destination exact peer/32 and fixed
port. lo can stay down: SYN must choose the TUN, not lo. Validate IP version,
header length/lengths, TCP SYN flags, checksum where applicable and exact source/
destination/port. No reply/full connect, WG or receipt claim. Stop/expiry and one
partial setup rollback need verified kernel absence before namespace teardown
and unchanged parent namespace baseline. Device EACCES/capability missing is
blocked/inconclusive, no chmod/sudo/install/fallback. No actual capability probe
was made here. A future capable runner must be named and separately released.

Crash stage is separate source/release: original child pidfd only, parent-death
launcher with post-arm parent check and supervising thread lifetime; kill one
owned child only after explicit release; held namespace FD allows postmortem
link/route absence without keeping a nonpersistent TUN alive. Verify all
inherited descriptors and child exec CLOEXEC behavior. Supervisor failure is
stop, not retry. No crash launcher or signal exists in this packet. Native
OS-TUN pair comes only after ordinary SYN and crash stages, separately reviewed,
with current finite authority unchanged. Useful attended duration remains the
separate policy draft, not extended max_lease/challenge/currentness here.
