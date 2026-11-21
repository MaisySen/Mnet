# S5 read-only bootstrap evidence driver, source for review

No execution release exists. This document records source/design and proposed
bounded evidence, never grants permission. Builder compiled the standalone
experiment and ran pure/fake tests only. No launcher/client/cap-drop/filter,
namespace, signal0, netlink or directory-errno collection has run here.

## Source corrections and decisive tests

Direct tests now refuse missing AF_SPEC, unknown family, missing IPv4 family,
zero parent dev/inode and wrong securebits value. The tested exact securebits
constant is also required at its Prctl call, not a disconnected test constant.
The earlier six-bit securebits mask remains47. Existing S4 data and outcomes are
unchanged. No new kernel collection was needed to write these tests.

## Client signaling boundary

One-uid unprivileged userns maps owner and client to the same underlying kernel
uid. A nested userns remap would not change that fact. Capability drop and
nondumpability alone do NOT stop kill/tgkill of owner by same-uid client.

For this fixed read-only evidence client, source now installs a persistent
x86_64 seccomp barrier before fixed final exec. It denies kill, tkill, queued
signal variants and pidfd_send_signal; tgkill allows only its own tgid with
SIGURG for Go runtime preemption. It also denies ptrace/process_vm access,
pidfd_getfd, unshare/setns, clone3 and io_uring_setup. clone accepts only the
exact ordinary Go runtime thread mask0x50f00 with high32 bits zero. Wrong arch
and x32 are killed, not tolerated. This mask is grounded in the current Go
runtime's runtime/os_linux.go cloneFlags, not asserted portable. Unsupported
architecture/runtime/filter installation is fail-closed before client work.
Filter is inherited across exec and into subsequent threads. All old threads
are replaced at exec after the cap drop. NNP, zero caps/bounding/ambient,
securebits, CLOEXEC descriptor allowlist and owner nondumpability still apply.

Pure classic-BPF interpreter tests evaluate exact owner/group/negative pid,
alternative signaling, wrong-architecture/x32, namespace and clone boundaries,
plus ordinary runtime calls and self-preemption. These are filter-byte tests,
NOT actual seccomp syscall evidence. A real bootstrap must also show one extra
Seccomp_filters count versus the pre-install count, Seccomp2, five zero cap
sets and NNP1. A preexisting platform seccomp2 alone is not accepted.

This resolves the proposed direct signaling policy only for the fixed evidence
client. It is NOT a general hostile-app or filesystem sandbox. Same-uid access
to user-owned files, cgroups, ordinary filesystem side effects and arbitrary
future executables are not claimed isolated. Driver runs only audited fixed
code, passes no private files/env, and never publishes an ordinary-app consumer.
For later real clients, prefer a runner with a separately mapped non-owner uid
or a separately reviewed stronger sandbox. That capability is not established;
no sudo/newuidmap/install/host fallback is proposed or executed.

## Standalone read-only source driver

Explicit build tags mnet_enrollment_development,mnet_os_readonly_evidence select
experiments/osownership/cmd/bootstrap-evidence. Product builds have no new
entry/flag/caller. A distinct readOnlyEvidenceBuild enables only read-only socket,
namespace inspection, sysctl reads and fixed bootstrap paths. The original
kernelExecutionReleased remains false and TUN, Apply and IPv6 write constructors
still refuse; structural tests check these boundaries in the evidence build.

Proposed outer mode --bootstrap-readonly performs:

1. Record numeric initial namespace, spawn fresh user+net namespace with the
   reviewed pinned-thread startup gate and pidfd/owning-userns checks.
2. Resident reads two equal complete raw link/address/route/rule inventories;
   exact AF_SPEC/directory collector records actual per-device present or ENOENT,
   never converts EPERM/ELOOP to absence. Validate full immutable baseline and
   forwarding/rules/applicability before client start. Unknown output blocks.
3. Spawn exactly one fixed client through the actual cap/NNP/filter/CLOEXEC
   bootstrap path, not a mock helper. Client emits allowlisted status fields,
   fd numbers, uid/pid/ppid and namespace identity. It attempts owner signal0
   only (no delivered signal); EPERM is required. Combined with filter count,
   this exercises the barrier without a termination/stop/crash signal. It does
   NOT attempt unshare/setns, network connect, device open or networking writes.
4. Join client on the still-pinned owner thread. Capture full raw inventory and
   sysctls again. Require exact baseline unchanged, no resource mutations.
5. Resident emits one <=64KiB final verdict and exits. Outer joins it, validates
   same client namespace, zero caps/NNP/filter/FD evidence, explicit client join,
   owner/outer distinctness and unchanged outer namespace, then emits result.

Client stdout/stderr each capped8192 bytes, 250ms pipe wait reserve. Resident
budget4.5s inside outer5s supervisor, top-level6s. Inventory retains250ms/2
attempt bound. No automatic retry of the overall attempt. Existing parent-death
and timeout cleanup source can kill/reap only owned children; this is not a
network crash test. An unreaped owner retains namespace custody as pending.
If any check fails, preserve the failure and original outputs, do not relax a
filter, widen a decoder, retry or fall back to the host within the attempt.
No statement of absence is inferred from an empty pipe or process exit alone.

## Gates and proposed evidence scope

This is source to review before choosing a named runner and an exact one-shot
execution release. Review the filter, build-tag boundaries, launcher/custody,
client FD collection, signal0 probe and every failure/reap path. Actual dynamic
FD inheritance/thread-churn/pidfd/capset/filter behavior remains unobserved.
Compile output is labeled COMPILED-NOT-RUN and is not shipped in the packet.

If separately released, retain executable/source hashes, command, stdout/stderr,
exit/reap, all explicit error classifications and parent/child identities. One
failure stays a failure, no retry. This proposed evidence does not authorize
any link/address/route/sysctl mutation, TUN/open, traffic, WireGuard, trusted-pair
repeat, escalation, service activation, real app client or production use.
