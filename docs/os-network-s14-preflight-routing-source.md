# S14 separate preflight routing, source only

S13 no-dispatch limitation is superseded by this dedicated standalone entry.
No live invocation, target metadata/device access, namespace/filter/probe or
mutation occurred. Existing BootstrapEvidenceMain four modes remain byte-unchanged;
fixed client code/filter unchanged; kernelExecutionReleased remains false.

## Exact mode graph

Separate cmd/device-preflight with all four tags, modes ONLY:
--device-open-preflight -> unprivileged outer -> reviewed SuperviseResident ->
fresh user+net namespace -> --osownership-resident-device-open-preflight ->
held namespace/nondumpable owner -> runDevicePreflightSource -> bounded verdict.
No fixed client is spawned or invoked. No ioctl, TUN, network write or fallback.

Resident launcher parameterizes only these fixed mode choices, with build-tag
allowlist before any open/spawn. Original residentSpawner/entry still routes
exact original mode. No callback/path/env injection route added. Existing pidfd
before Wait, pinned starter through reap, child namespace ancestry, startup gate,
CLOEXEC verdict custody and cleanup handling reused. Tests were updated to point
to extracted underlying source bodies, plus new forwarding/source wiring guards,
not deleted. Both wrappers retain old default route.

Strict outer decoded verdict validates device evidence, exact expected owner
namespace, matching baseline hashes, namespace/FD rechecks, applicable outcome
fields, actual reap/custody-close and outer namespace before/after unchanged.
Feasibility-recorded is not device/TUN success: missing/denied/refused-object are
legitimate observations after unchanged oracles; open success only runner node
access. Errors stay errors, no retry. Device evidence types moved unchanged to
portable development file for strict typed Verdict field, no raw payload bypass.

Owner baseline compares raw read-only inventory/sysctl bytes within-run. No
outer NETWORK INVENTORY hash is captured, only outer namespace identity; don't
call that proof of whole-host unchanged networking. Volatile raw fields may
false-fail, no normalization or cross-run claim. Real kernel adapter remains
unimplemented/unwired, not silently completed by this preflight.

## Four surviving tests closed

New injectable owner orchestration seam is used by REAL source with held
identity, concrete ops and real read-only snapshot closure. Fakes inject two
different raw hashes and nonisolated namespace before any operation. Independently
decisive tests now kill removed major check (wrongmajor SAMEminor), forced final
NamespaceRechecked=true (errno retained still must flag false), removed isolated
refusal (no snapshots/opens permitted), forced BaselineUnchanged=true (different
hashes must return error). All four compile-valid/uncached mutants killed.

Additional routing mutants: removed outer namespace equality, custody-close
guard and fixed mode forwarding killed. A removed outer baseline-binding check
initially SURVIVED, then added mismatching outer/payload test and reran, now
killed. Logs preserve both discovery and closure, originals restored.

AST call allowlist is only a tripwire, not proof of arbitrary future helper
purity. FD inventory equality is number equality, not description identity and
can miss same-number replacement. Opened device fstat identity is separately
checked. Path ancestor symlink/replacement risks remain despite final NOFOLLOW;
this source does not claim a hostile-filesystem sandbox. Ordinary TUN guard
still refuses. No ioctls hidden in new effects inventory.

## Wrapper source and bounds

Complete Python wrapper CLI takes exact binary/hash and NEW output directory,
no guessed defaults. Nonzero uid, absolute nonsymlink regular binary prehash,
post-launch /proc/exe rehash, fixed only --device-open-preflight and minimal env.
This is SOURCE for review, not permission to run. Single-user writable-path
prehash/exec race remains: hash is not atomic fexecve custody.

Includes full snapshot library from S13 for review. Tgid, bounded cmdline,
stat-start recheck, per-record/per-children-read and collection timestamps.
Unreadable/error retained UNKNOWN, never absence. Task-leader children only,
non-atomic exec/identity samples; no retrospective937 attribution. Only owned
fixed no-secret argv, no arbitrary-process gathering.

Wrapper preserves full available outer stdout/stderr and all tree records;
waitid WNOWAIT observes driver exit before Wait, avoiding releasing its PID just
to poll. On10s timeout/identity failure, only exact fresh PID/start/exe/Tgid
comparison before driver SIGKILL, no name/group cleanup.2s Wait refusal is
pending_custody, never false reaped. Go6s outer/5s supervisor plus500ms cleanup
are cooperative observation/control deadlines, NOT a promise an arbitrary kernel
device call exits or a D-state child is killed within them. Abnormal process/
custody result must be handed back, not automatically retried/escalated.

Implicit effects ACTORS for THIS mode: outer driver first StartProcess may do
Go self-pidfd open/waitid-ECHILD/self-signal0/transient0x5100 clone-exit/reap/close;
0x5100 already includes PIDFD. Resident has NO StartProcess/client here, so no
resident per-process first-launch check assumed. Existing bootstrap mode still
has separate resident check when it starts fixed client, unchanged. Fixed client
not invoked here, remains nonspawning/filter unchanged. Pdeathsig cleanup inherited
from reviewed launcher; no claim that implicit effects were per-syscall traced.

8 Python tests purely offline: fixture snapshot, unknown read, inventory,
exact-lifetime mismatch, static scope/import, COMPLETE mocked launch and
refusal to signal changed identity. No subprocess/mock test actually launches.
Go pure/fake race, selected tagged race, vet/compile only. Exact preflight binary
is COMPILED NOT RUN. Before any execution decision, independent source and full
wrapper review, explicit exact-hash release and outcome/custody handling needed.
No user-machine access, install/helper or production activation implied.
