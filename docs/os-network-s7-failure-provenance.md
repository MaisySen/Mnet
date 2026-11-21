# S7 bounded failure provenance, source only

Original S6 one-shot failed and is immutable at evidence-only02d861c. Failure
cause remains unknown. No bootstrap rerun or new namespace/filter/capdrop was
performed. Only parent-released read-only survivor assessment inspected current
process records; no signals, process cleanup or invocation followed it.

## Original evidence limits, unchanged

Driver914 reaped exit1, outer netns unchanged. Resident stage+baseline hash is
source-consistent with initial live inventory/rules/sysctl applicability passing.
Raw per-directory statuses were not saved, so exact errno is not inspectable.
No cap/FD/filter/client success follows. cleanup_confirmed=false means final
success absent, NOT an established leak. Missing child diagnostics cannot be
reconstructed as an assumed runner incompatibility.

Later read-only survivor snapshot found no PID914 and no exact original binary
path/hash candidate among readable process records. Driver start ticks and
resident/client PID/start identities had not been recorded. A runtime restart
also occurred between attempt and assessment. Exact historical lifetimes and
complete absence of arbitrary reexec descendants remain unverified. The new
snapshot cannot repair the missing original process audit. Assessment is a
separate JSON, not an overwrite of original stdout/stderr/result.

## Diagnostic source changes for the next review

No secret-bearing arbitrary stderr or error string is put into a verdict.
Client main emits a strict diagnostic record: source-selected operation/class,
typed errno and phase. Raw Go error text is not forwarded. Existing stdout and
stderr remain bounded. Stderr retains captured byte count, SHA256, completeness
flag and only strictly parsed allowlisted diagnostic content. Unknown/malformed/
duplicate/trailing/private bytes get hash/length only, no text. Captured length
is not asserted to be total output when a pipe/limit fails; complete=false.

Source-selected raw stderr phase markers are bootstrap-entry, capset-zero,
filter-installed, final-entry and fd-validated. They avoid os.File poll init.
Owner accepts at most8 bounded records, each<=1024 bytes, known schema only,
and exposes last validated marker. Missing or malformed progress stays unknown,
never called seccomp-installed. Every dangerous bootstrap syscall now has a
source-selected operation with errno when available. DiagnosticForError never
copies arbitrary error strings or operation names. No filter/cap path changed.

Six reviewer visibility pins:

1. Operation + typed errno in bounded child diagnostic and resident failure.
2. Client exit code or signal, Wait/reaped status, stdout bytes/completeness and
   JSON-validity indicator even when the client exits nonzero.
3. Stderr class/phase/hash/captured length/completeness, never raw output.
4. Resident and client PID/start ticks, identity-observed flags and explicit
   reap/custody-close/retained booleans. Start ticks read before reaper starts.
   Unreaped remains retained custody. Future snapshots are evidence only from
   that future attempt, never retrospectively assigned to S6.
5. Initial IPv6 directory status map/link count/namespace evidence installed in
   Verdict.Bootstrap BEFORE starting client, hence retained on client failure.
6. Client own failure class plus phase markers distinguish the last observed
   stage; absence is unknown. Successful client lifecycle is joined explicitly.

Supervisor attaches owned parent-side lifecycle after handling the verdict,
not a child-supplied reap claim. It records abort/reap/close attempts separately;
failed reaping keeps custody and does not close it. Spawn pending errors retain
an available process snapshot rather than losing identity. Networking final
cleanup success remains separate from process reaping and custody closure.

Tests are pure/fake/structural: each operation maps to safe diagnostic/errno;
raw private errors do not appear; strict stderr parsing rejects unknown/trailing/
duplicate/incomplete content; phase+diagnostic chains retain known facts; fake
supervisor failure/success/pending paths retain their own lifecycle; parser
handles PID/stat comm delimiters and rejects malformed/reused PID identity.
Evidence-build structural tests require six retention sites before client start
and on failures. These tests do NOT exercise actual child failure/cap syscalls.

Source build compiled but NOT RUN. Original evidence-tag3fbe01e binary remains
unchanged; new compile has a distinct path. Compiler Go1.26.6 temporary modfile,
not exact repository Go1.26.11. No TUN/netlink/sysctl writes, new capture, signal0,
new filter/client/namespace run, retry or activation. Another execution decision
requires independent exact-source review and separate one-shot release. Narrow
fixed-client threat model and general-app containment gaps remain unchanged.
