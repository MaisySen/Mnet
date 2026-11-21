# S11 exact Go amd64 TLS clone correction, source only

Third result remains FAIL, prior failures unchanged. The deterministic source
mismatch is now corrected: normal Go1.26.6 linux/amd64 newosproc base0x50f00
plus assembly CLONE_SETTLS0x80000 yields exact syscall flags0xd0f00. Filter
requires highword0 and lowword0xd0f00. Former base0x50f00 is refused, no separate
nil-mp/gp path required by this fixed client. No broad masking/extra flags.

Runtime gate now requires linux/amd64/go1.26.6 before constructing filter. Pure
characterization requires same version and exact SHA256 of runtime/os_linux.go,
sys_linux_amd64.s and proc.go, checks assembly flag-load/non-nil/TLS addition,
and derives the accepted mask from symbolic thread constants plus SETTLS rather
than just copying base magic. A compiler/runtime source/version change fails
characterization, requiring new review. Actual compiled binary disassembly also
preserved, not executing it. Repository go.mod remains1.26.11; local temporary
modfile selects1.26.6. Therefore an ordinary build under another compiler must
fail this experimental characterization/gate until separately reviewed.

Tests refuse base0x50f00, every one-bit removal/addition across64 flags bits,
including namespace/parent/ptrace/exit-signal/high-word changes. Existing signal,
arch/x32, privilege, fixed dispatch and bounded diagnostics tests remain. Six
compile-checked uncached mutants killed: old base mask, highword bypass, extra
lowbit, lowflag bypass, version bypass and pin-not-applied. Original restored.

Stack/tid/TLS argument values are not newly checked by current filter. With the
exact flags, tid words request no tid operations; TLS is an ordinary thread TLS
pointer. This remains a narrow fixed-client signal barrier, not arbitrary-app
containment. No extra syscall grants, clone3/namespace/ptrace/parent flags, caps,
securebits47/NNP changes, raw stderr retention or policy fallback were added.

Earlier reviewer/base-mask correctness statements are withdrawn. Concrete
mismatch is consistent with third observed newosproc errno1 but neither sole
historical cause nor before/after-exec stage is proved. No cap/NNP/filter/FD/
signal0/baseline/final-cleanup success from any failed run. Within-run raw hash
may false-fail from volatility; no cross-run equivalence/normalization claim.

Pure/fake race and selected evidence-tag race, vet and build only. No fourth
bootstrap, namespace/filter/probe or mutation execution. Await exact-source
independent review and separate live authority, no automatic next run.
