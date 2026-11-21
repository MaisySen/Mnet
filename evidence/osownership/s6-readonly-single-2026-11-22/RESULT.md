# Single released read-only bootstrap attempt: FAIL

Exact source3fbe01ed298c7fc77681f0f99e4e303a56610aed, evidence-tag binary SHA256
0164f1dbdaa63bd9379dfe92948089a142f4c3a389d28463f481b19c898feefd.
Go1.26.6 temporary modfile, not repository-pinned Go1.26.11.

One --bootstrap-readonly invocation, outer uid1000. Driver PID914 exited1 and
was reaped in16.164408ms. No retry. Outer namespace dev7/inode4026531833 remained
unchanged. Resident verdict identifies distinct dev7/inode4026532293, reached
client-bootstrap with baseline hash147cd885270866196875df5dd086fe093a5c8c4e08ee2620c15cfb36dabcdc35,
then failed class runner. Final hash absent; cleanup_confirmed=false.

This is NOT bootstrap success, filter proof, descriptor proof, capability proof,
IPv6 errno proof or independent cleanup confirmation. The source successfully
advanced beyond initial raw baseline validation, but the per-directory evidence
is not carried in this failure verdict, so it is not independently inspectable.
No mutation/TUN/kernel teardown/containment/product/networking claim follows.

Evidence limitation: runReadOnlyClientAndJoin buffers child stderr but does not
forward it, and residentBootstrapSource drops the actual work error except its
class. Therefore underlying client-bootstrap failure cause and exact child
pid/lifetime checkpoints were NOT preserved in the available verdict. The outer
supervisor's error lacks pending-resident custody, but that is not a full child
lifetime audit. Do not retrospectively infer a syscall error or missing FD from
this generic failure. All available raw stdout/stderr and outer process result
are preserved unchanged. No second probe/run was performed.

Next source-only diagnostic improvement for review: retain allowlisted failure
stage/operation/errno, client exit/signal and explicit child reap/identity in the
bounded verdict, without private stderr dumps. This note proposes no execution.
