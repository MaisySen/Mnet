# Single sandbox existing-device preflight: DENIED, valid observation

Source c7699b3. Exact binary6eaa51b9a258d86a7be2107f165c292c6260ee34a8269adc97d61a0386b60ab6,
wrapper4051a9a79d8a1671319197aa75f9f5333eccf3da25e2da3d007d17f37bfc4136,
snapshot0340996e5a1fb4c14c9bafb844f5adf2a7717fd73ccc4cbc4ef44d69c3ee69ca,
all rehashed immediately before ONE invocation. Go1.26.6 temp modfile, five
explicit tags, repository go.mod unchanged1.26.11. Compiler metadata in artifact
report, executable filename COMPILED-NOT-RUN is historical and this exact artifact
HAS NOW RUN ONCE. No retry or automatic next gate.

Outcome feasibility-recorded, not node access success/TUN support. Existing
/dev/net/tun lstat: device9/inode134/rdev2760 char10,200 mode8576 decimal
(char0600), inner uid/gid65534/65534. Exact owner-side O_RDWR|NONBLOCK|CLOEXEC|
NOFOLLOW open returned EACCES13, FD=-1. No opened-FD fstat/fcntl/close applied
because open denied. Outcome fields false for CLOEXEC/NONBLOCK/closed therefore
mean NOT APPLICABLE, not failed teardown. Metadata alone cannot identify which
DAC/ACL/device-policy layer accounts for denial; no deeper probe or bypass.

Owner namespace dev7/inode4026532293 isolated, outer7/4026531833 unchanged.
FD inventory[0,1,2,3,4,5,6,7] identical, numeric-only inventory limitation.
Namespace rechecked. Raw within-run baseline/final equal
5fce6d503b9e0b491fe16e487f1ab4ba3b89bfbcea28d58f954a9e8afd6152a0.
No outer NETWORK inventory hash, only namespace identity. No cross-run equality
or volatility normalization implied. Resident verdict final/pass here means
valid unchanged feasibility collection, NOT successful device open or creation.

Driver939/start3088 exit0/reaped, resident944/start3088 owner-recorded reaped,
custody closed=true/retained=false.14.79505ms, stderr empty. Wrapper sampled
resident PID through children but its identity read was already not-found;
PID/start recovered from bounded owner lifecycle rather than fabricated cmdline/
Tgid/exe. Both exact recorded lifetimes absent-at-read immediately afterward,
no unknown postchecks/survivors. Sampling non-atomic/task-leader-only, not all
process or stdlib syscall attribution proof. Explicit driver implicit-effects
inventory is predicted from pinned Go source, not per-syscall capture.

No TUNSETIFF/ioctl/create/traffic/netlink/sysctl write, escalation/install/
fallback or user-machine access. Sandbox denial is ONLY this runner at this
instant, not user's machine feasibility. All prior3 bootstrap failures, fourth
read-only success and accidental mutant namespace incident remain unchanged.
No new scan or probe after capture. Parent decides next architecture gate.
