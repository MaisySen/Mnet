# S6 read-only driver FD harness correction

Source review checkpoint only. No driver/client/namespace/filter/capability or
networking operation was executed. S5's generic Go file APIs could initialize
runtime eventpoll/eventfd before FD validation, falsely failing strict0/1/2.
This is a harness defect, not evidence of an inherited leak.

The final fixed client now inventories descriptors FIRST using unix.Open,
Getdents and ParseDirent. It excludes only the exact directory descriptor it
just opened, under its own custody; duplicate/missing/malformed entries fail.
Raw bounded unix.Read collects status; raw open/fstat collects namespace. A
second raw inventory before JSON/stdout publication must still match the first.
There is no os.File, os.ReadFile, os.ReadDir or os.Readlink between these checks.
The standalone final entry path performs no ordinary app/socket work first.

Strict FD0/1/2 remains. No eventpoll/eventfd number, count, inode, name or generic
CLOEXEC exception is allowed. If an unrelated Go startup behavior creates a
preexisting descriptor anyway, the result stays a HARNESS failure with ownership
unverified; no descriptor is retrospectively called inherited or runtime-owned.
The exact owned getdents directory FD is the only positively attributed exclusion.
The pre-exec bootstrap may initialize runtime descriptors, but its CLOEXEC
close-range and fixed exec remove them before final-client entry. This is source
reasoning, not dynamic FD proof.

Client unexpected-FD output carries bounded harness-fd-inventory classification.
Owner parses that even on a nonzero client exit and reports harness failure,
not a false safety verdict or silent pass. Failure classes remain in the bounded
resident/outer result. Existing read-only/TUN/mutation build gates are unchanged.

Tests use pure dirent-name inputs to prove exact owned-FD exclusion, foreign
fd3 refusal, eventpoll-like extra descriptors refused, missing owned-directory,
duplicate and malformed entry refusal. Source ordering checks forbid poll-
registering Go file APIs and require raw inventory before status collection.
These structural tests do not exercise Getdents or prove actual FD inheritance.

New decisive filter tests hold SIGURG constant while changing tgkill target to
owner/zero/negative/high32 cases, and individually cover unshare,
process_vm_readv and pidfd_getfd denials. Compile-checked, uncached source mutants
remove each of those three denies and tgkill target equality; each fails its
named test. Independent zero-parent dev and inode mutants also fail the direct
component test. All six mutant logs are retained; source restored byte-identical
before final race tests/build. No filter was installed or run.

The previously proposed exact one-shot read-only scope is unchanged: fresh
user+net namespace, fixed cap/NNP/filter client, actual directory errno collection,
no TUN, mutation, sysctl write, connect, host fallback, retry or real app client.
Signal0 is only a proposed no-delivery filter probe. Needs independent corrected
source review and a separate execution release. Compile is not permission.

Same-uid filesystem/cgroup/unix-socket containment is still out of scope. The
filter is a narrow signal/ptrace/namespace barrier for the audited fixed client,
not general hostile-app sandbox proof. Real thread/pidfd/capdrop/FD/filter/errno
behavior remains unobserved until separately released evidence.
