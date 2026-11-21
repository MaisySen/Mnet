# Proposed no-TUN conformance, not released

First target: named disposable Linux namespace runner already able to create
an isolated user+network namespace, lo and dummy. No TUN device, WireGuard,
transport traffic, user profiles, approval consumption or production daemon.
No sudo/chmod/install/host fallback. Runner capability is unverified.

## Required process placement before any execution

Outer parent retains only target namespace FD, child pidfds, one bounded verdict
pipe and bookkeeping. It must not own or read a TUN FD. Parent captures host
namespace identity/baseline read-only and refuses initial-namespace equality.
The exec-created owner lives entirely in its private user+network namespace,
locks its OS thread for namespace-sensitive operations and never setns back.
Namespace identity comes from held nsfs FDs, not a pathname/tag assertion.
A separately exec-created client PID is in the same target namespace; it never
holds the owner's resources. No threads or subprocesses may escape ancestry.
Namespace lifetime/custody must remain held through final readback.

This is placement design, not implemented supervisor source. A review must
settle userns ownership/ancestry proof, namespace-FD duplication and child
reaping, ptrace/process-exit handling and deadlines before a driver is runnable.

## Bounded evidence contract

One run only after exact-source release. Fixed total supervisor deadline, fixed
operation deadlines and cleanup reserve chosen in driver review, not a new
runtime grant. Pipe is one bounded structured record (max64KiB), exact version,
stage, namespace identities, sequence, errno, resource identities, baseline and
final summaries. Any missing/duplicate/oversize record, pipe EOF, timeout,
unknown result or unreaped child fails. Never equate process disappearance or
pipe closure with networking absence. No private keys/tokens in verdicts.

## Proposed finite stages

1. Verify runner namespace support read-only; if unavailable, fail with facts.
2. Spawn contained resident owner; prove held target differs from outer parent.
   Snapshot all tables/rules/address/link/sysctl state twice completely. Keep
   unknown attributes and reject unexplained state, not selective lo-only view.
3. Read lo baseline. Create one exact uniquely named/aliased dummy exclusively;
   record ACK and full readback index, kind, flags and attrs. Name collision
   EEXIST must fail, never adopt or delete preexisting resource.
4. Disable IPv6 on that link using explicit per-link sysctl/readback. Add one
   fixture /32, bring up, add exact second /32 route with proto242/table254/
   priority42700. No default route, broad mesh route, policy or forwarding edit.
5. Verify exact identities, flags and own/peer destinations across ALL tables.
   Remove exact route/address with fresh identity+namespace checks before each
   mutation. Delete dummy by exact original index/identity, not TUN FD semantics.
6. Verify both fixture destinations/addresses/link absent and complete baseline
   unchanged. Verify outer namespace unchanged. Reap all children; close custody
   only after final verdict. Interrupted dumps discard whole snapshot, retry
   only within bounded inventory policy, then fail pending, never partial pass.

Future failure cases must be separate approved contained runs/source: namespace
swap/refusal, exclusive collision, interrupted dump, partial success timeout,
foreign replacement, cancellation. No crash signals are released by this plan.
Dummy RTM_DELLINK tests neither nonpersistent TUN close nor owner-death cleanup.

## Later TUN/pair sequencing boundary

A later pair must stop accepting new clients, stop/join traffic consumers and
WireGuard, then enter the finite route-cleanup window. No application traffic
continues on cleanup-only authority. The resident owner alone reads its TUN FD.
A distinct outer supervisor timeout never supplies networking authority or a
longer runtime lease. Crash, expiry and ordinary teardown need independent
real evidence. Existing successful userspace pair evidence is not this proof.
