# S9 source-only runtime stderr classification

S6 and S8 failed attempts remain immutable. No third run, new filter/capdrop,
namespace/process probe, signal0 or networking action. S8 client exit2 underlying
cause remains UNKNOWN. Its1400-byte raw stderr was not persisted and cannot be
reconstructed from its hash. No filter policy or bootstrap ABI was weakened.

## Source investigation

The standalone dispatcher has four fixed mode strings. Owner client-start uses
--osownership-client-capdrop-bootstrap-source; bootstrap exec uses
--osownership-client-unprivileged-source. Both match actual switch cases.
Resident reexec mode matches too. Any ordinary error from this dispatcher/main
emits a sanitized JSON diagnostic and calls os.Exit1, not2. The candidate package
has no init function. These facts rule out an intentional exit2 in this fixed
main, NOT failures in runtime/init/dependencies or any particular syscall cause.

Local Go1.26.6 runtime/panic.go fatalthrow/fatalpanic paths call exit2.
runtime/os_linux.go newosproc can emit an exact failed-thread line and throw
newosproc. These are examples supported by source, NOT a diagnosis of S8. The
1400-byte length/exit2 are insufficient to select one. Fixed-mode tests pin
matching dispatch and source exit convention without executing a child.

The actual diagnostic blind spot is source-readable: scanSanitizedStderr rejects
an entire mixed stderr stream if any unknown/runtime stack line exists. Thus
phase=unknown does not establish that no earlier source phase marker ran. It
only establishes the whole strict stream failed parsing.

## Narrow future classification

A separate bounded classifier now preserves only a strict LEADING prefix of
allowlisted phase records (<=8, each<=1024 bytes, within8192 captured bytes).
After first unknown line, later phase-like JSON is never trusted. No source
instruction, new goal or parameter is taken from stderr.

It recognizes exact fixed runtime token newosproc; exact failed-new-thread line
with bounded nonnegative thread count and errno1..4095; exact deadlock fatal
line; or the generic panic-prefix token without its value. It outputs ONLY an
enum token, bounded errno, recognized-line count, unparsed-bytes flag and last
strict source marker. No stack frame/path/address/goroutine text, panic value,
private string, arbitrary line or environment is retained. Existing captured
hash/length/completeness remains authoritative. Incomplete stream is explicitly
classified incomplete, not a complete diagnostic. Unknown data remains unknown.

A known runtime token is an observed print classification, not proof of why it
occurred (filter, limit, runner etc). Last strict marker is last observed phase,
not proof later phases failed or succeeded. The old strict sanitized-diagnostic
parser still rejects mixed/private text; the new summary is independent and
cannot make unknown child status a successful bootstrap.

Pure synthetic tests exercise marker+fatal+private stack, no private reflection,
late/malformed marker rejection, exact token/errno bounds, incomplete stream,
panic-prefix redaction and marker-only behavior. Evidence-tag structural tests
check fixed dispatch. Actual runtime fatal, filter and child behavior remain
unexecuted. No claim that synthetic stderr matches the lost S8 bytes.

Only after independent exact-source review could a separate bounded read-only
execution decision be considered. No rerun is granted here. No change to
same-uid narrow fixed-client threat model, mutation/TUN holds or product state.
