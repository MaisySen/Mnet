# S4 correction source, not live conformance

Supersedes the S3 applicability blocker and unsafe launcher/client statements.
No kernel networking, namespace, capability-drop, process launcher, parent-death
signal, sysctl or TUN execution was performed. Guard stays compile-time false.
No handler or stage executor has been wired. Source/fake/ordinary-file tests only.

## IPv6 evidence and source rule

Recorded run2 AF_SPEC family10 exists exactly on the seven links whose IPv6
sysctl directories appear in the archived complete sysctl listing. Three IPv4
fallback links have only family2 and no IPv6 directory. The earlier blanket
per-link IPv6 requirement was wrong for this captured kernel/runner.

Applicability now follows the same stable raw link AF_SPEC family set. Missing
AF_SPEC, malformed nested framing, unknown families or missing IPv4 state fail.
Applicable links require a present directory plus valid disable_ipv6,
addr_gen_mode and forwarding leaves. Inapplicable links require directory
ENOENT and no IPv6 leaves. This is not IPv6-disabled evidence. all/default remain
required. Collector source performs exact no-follow directory open, distinguishes
ENOENT from EPERM/ELOOP/other errors, and checks the namespace around reads.
Stable raw dumps and exclusive custody remain required around sysctl collection.

Offline fixture normalization derives presence/absence markers from the archived
sysctl listing, without modifying the original JSON. That listing did not record
individual directory-open errno values. Tests establish rule consistency with
captured families/listing, NOT that the new ENOENT collector was run. A future
reviewed read-only confirmation may exercise it; none was run or requested here.

Baseline source now takes separate parent and child numeric identities and
rejects equality rather than comparing the child to itself. Nonzero IFLA_LINK
underlay linkage, master/cross-netns attrs and veth/vxlan/ipvlan/macvlan/macvtap/
geneve kinds fail, including those with zero IFLA_LINK. No name allowlist. Exact
raw per-run baseline remains unowned and immutable. The isolation proof, absence
of NIC/veth linkage and an incapable client are the trust boundary, not kind or
name alone. This is still a source threat-model proposal, not broad approval.

## FD, pidfd and thread correction

Resident receives only verdict fd3. Initial parent network namespace identity
is numeric dev/inode, not an inherited FD. Child immediately sets verdict/gate
CLOEXEC and checks parent alive before and after startup gate; owner becomes
non-dumpable before client start to prevent same-uid client /proc FD access.

The parent starter goroutine pins its OS thread before cmd.Start and keeps it
pinned through child reap, including retained-custody failure outcomes. This
matches Linux Pdeathsig's creating-thread lifetime. Pidfd acquisition is before
any cmd.Wait/reaper starts, so an exited unreaped child still reserves its PID.
Actual pidfd/userns/TID/parent-death behavior remains unexecuted source.

## Client privilege boundary source

Client start is allowed only within the resident's locked-thread custody. Its
fixed bootstrap verifies parent alive, sets no-new-privs, clears ambient caps,
drops every bounding capability, locks NOROOT/no-setuid-fixup securebits, clears
permitted/effective/inheritable sets, marks descriptors3+ CLOEXEC, then execs one
fixed final client mode. No app callback runs between drop and exec, and exec
replaces all old threads. The final entry must verify all five capability sets
zero, NoNewPrivs1 and descriptor allowlist0/1/2 before client work. No TUN,
verdict pipe or namespace FD may reach the client. Root uid alone cannot regain
caps with zero bounding set, locked NOROOT and no-new-privs.

This guarded bootstrap and final verifier are SOURCE, not an exercised sandbox.
No actual final handler is wired; no claim that a currently shipping client is
unprivileged. The owner must join client before leaving its pinned scope and
before cleanup. Missing handler/client join wiring remains a release gate.

## Tests and limits

Added independently decisive baseline-up/master/underlay/crossns checks,
per-link IPv4/IPv6 forwarding, changed-but-valid baseline sysctl, explicit
parent equality, AF_SPEC/directory mismatch, inapplicable leaf refusal, exact
owned address header/extended flags, TagConfirmed before up, final digest
mismatch with nil cleanup error, rule count, cross-namespace link kinds, and
client cap/no-new-privs status cases. Source AST/text ordering tests pin pidfd
before Wait, starter-thread lifetime, CLOEXEC/numeric identity/parent checks and
cap-drop-before-exec. These structural tests cannot replace real descriptor,
thread-churn, syscall or unprivileged-client execution tests.

Compiler remains Go1.26.6 temporary modfile, repository go.mod unchanged. Logs
record package-only race/vet. Existing captured read-only outcomes preserved.
No repeated pair, live conformance, TUN, crash, grant extension or activation.

Remaining before any live mutation: independent full-source review; applicable
owned dump schema and exact adapter; stage executor and gated entry handlers;
real unprivileged-client/descriptor/thread lifetime evidence under a separately
scoped release; baseline containment review and named runner; one-shot explicit
conformance release. No new read-only collection needed to author this source.
