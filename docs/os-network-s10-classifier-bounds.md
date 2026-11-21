# S10 bounded classifier completion, source only

No third read-only run or new namespace/filter/client/probe. Both original
failures remain unchanged and their underlying causes UNKNOWN. No raw private
stderr retained, no filter/privilege policy weakened.

Five formerly surviving bounds now have decisive tests at exact edges: errno0
and4096 refuse, thread-count1000001 refuses, ninth phase marker cannot replace
the eighth, line1025 cannot act as a marker while1024 remains valid. Five
uncached compile-checked mutants remove each bound under the evidence tag. All
fail the named bound test; source restored before final verification.

Classifier now emits fixed booleans for generic fatal-prefix, unexpected-signal
prefix and signal-header prefix; bounded goroutine-header count and unparsed
line count. No suffix, panic value, stack, path, address or arbitrary error text
is copied. Recognized-line count counts each line once, including overlapping
exact/generic tokens. Unknown/malformed tails remain unparsed; incomplete stream
cannot masquerade as complete. Generic tokens classify observed text only, not
filter/runner/syscall cause or successful bootstrap.

A single strict LEADING ChildDiagnostic record now survives a later unknown
stack/trace, along with the strict leading phase prefix. No diagnostic/phase is
accepted after unknown bytes. Exact known schemas, allowlists, trailing-data
refusal,<=8 prefix records and<=1024 bytes per record remain. Tests cover the
leading diagnostic+unknown tail and refusal of late diagnostics, with private
synthetic suffixes never present in serialized evidence. Real lost S8 stderr is
not simulated or reconstructed by these fixtures.

Raw diagnostic-retention decision: not proposed for this checkpoint. Fixed
classifiers now cover generic fatal/signal/goroutine shape plus exact bounded
newosproc and strict leading diagnostics. That source can be reviewed without
retaining private output. If a future separately released run still lacks enough
information, propose a distinct tightly local bounded retention scope for this
fixed no-secret client and review it BEFORE changing capture. Nothing here
creates such permission or stores raw child output.

Baseline hash clarification: JSON of raw within-run inventory+sysctls includes
stats/counters/AF_SPEC cache fields. It is byte-equality only within one run,
not stable identity or equivalence across namespaces/boots/runs. Volatile values
may cause safe false failure even without policy changes. No speculative field
normalization was added. S6/S8 differing hashes imply no cross-run defect or
policy change. A final within-run mismatch would need preserved source evidence
and review, not silent filtering or retry.

S8 exact driver/resident/client lifetimes are accepted only within recorded
sampling/reap limits, not arbitrary descendant or broad containment proof.
Compiler Go1.26.6 temp modfile, no repo toolchain change. Pure/fake race, selected
evidence-tag tests and compile/vet only. Another live step remains held pending
independent exact-source review and explicit release.
