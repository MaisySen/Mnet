# Native reliability investigation, 18 November 2026

Scope: standalone Mnet, commit 4beeabb. No Multies, deployment, production permissions, resolver bypass, timeout increase or release change.

## Current verdict

Not ready. The original owner-process DNS and independent-app failures remain failures. No production repair has been established. Stop repeating the batch native run unchanged.

- Native: https://github.com/MaisySen/Mnet/actions/runs/44229557229
- General: https://github.com/MaisySen/Mnet/actions/runs/44229557349

API34: original reconnect UDP receive timed out. Fresh connected and unconnected contrast queries passed, and independent OS/UDP/TCP DNS and bidirectional TCP hashes passed. Owner bulk transfer, consent, actual control, revoke and stop completed.

API36: the owner flow passed including initial/reconnect DNS and bidirectional hashes. The independent app's UDP send returned a permission-denied category and OS DNS failed; its numeric mesh TCP hashes passed. Consent, actual control, revoke and stop completed.

The general run passed four jobs, but the Go job failed the strict artifact-list test because this experiment added kernel-observation.json without updating that expected set. The test is corrected separately, with privacy and passive-observer assertions. That correction does not repair native networking.

## Discriminating evidence

The empty engineering emulator accepted a root AF_PACKET observer without app permission changes or SELinux changes. It logged derived fixture packet categories only, not payloads, addresses, enrollment keys or raw OS dumps.

API34:

- New active VPN network handle and interface were published at reconnect. It was not the old network identity in this reproduction.
- Reconnect original DNS began at elapsed 71264 ms and failed at 76275 ms.
- No fixture DNS reply was observed in that interval. The next replies arrived at 76282 and 76283 ms, after the failure and during fresh contrasts. Without the original socket's port and an outgoing capture, those cannot be matched to every originating query individually.
- All observed fixture DNS replies had valid IP and UDP checksums. Kernel InCsumErrors stayed zero. This does not support the theory that a malformed observed reply caused this failure.
- Capture reported 101831 packets and zero capture drops.
- The last UID-rule sample before reconnect DNS showed no matching UID-range rules. Matching rules appeared in the next sample at 71960 ms. Sampling was roughly one second, so this is evidence of asynchronous publication, not proof that the failed query preceded the exact rule installation.
- All four observed TCP flows finished with FIN and no reset. The intermittent reverse bulk timeout did not reproduce here.

API36:

- New network handle and interface were published at reconnect, and the owner's DNS replies had valid checksums.
- Independent UDP send permission denial remains an independent failure. It occurred later than initial publication; readiness timing alone does not explain it.
- Capture reported 237 drops among 86197 packets. Missing-packet conclusions and payload-byte totals are therefore not reliable for this API run.

Both runs accumulated system-wide UDP RcvbufErrors during bulk traffic. These counters cover encrypted transport and other emulator traffic. They do not prove that the original DNS packet was dropped for buffer pressure.

## Limits and ruled-down candidates

- This protocol-specific AF_PACKET socket observed incoming TUN replies, not outgoing TUN queries. It cannot establish whether the original failed query crossed the TUN boundary. Lifetime daemon booleans also cannot establish that per-query fact.
- UID rule table names were nonnumeric, so the numeric table allowlist returned -1. Only rule membership/count observations are usable; the table identity comparison is unresolved.
- Equal NetworkAgent identity alone would not be a fault: Android supports seamless handover. In this reproduction the identity actually changed.
- Go NewFile supports polling nonblocking Unix descriptors. A fresh Go1.26.11 nonblocking datagram-fd model waited through an empty read, received the exact packet and canceled a blocked read on close. Focused mobile fd tests passed under the race detector. This rules down a generic EAGAIN explanation, not every Android TUN-specific problem.
- Short TUN writes, checksum corruption and connected-only UDP behavior are not established causes. No speculative production fixes were applied.

## Concrete prerequisite for a safe repair

The next prerequisite is per-socket kernel-decision tracing on disposable Android34/36, not another success/failure-only batch run. An interactive KVM environment is the clearest route, but a deliberately designed trace-enabled CI experiment could also supply the evidence. The current packet observer does not. Required evidence at the same event is:

1. Original socket source port, current network generation, selected fwmark/netId and syscall errno.
2. Outgoing query at the TUN-read boundary, reply at the TUN-write/kernel-ingress boundary, and the kernel receive/drop decision for that tuple.
3. For permission denial, the actual netd/UID-policy or BPF/LSM decision, rather than inference from a published VPN capability.
4. If reverse bulk stalls, peer send progress, encrypted transport drop/queue state and both directions' TCP sequence/ack progress while stalled.

The local environment has neither /dev/kvm nor an installed emulator. Existing free CI can boot a disposable emulator, but the present harness retains only a completed packet-category trace, not the socket mark and policy/drop decision. Whether its images expose the required tracing hooks has not been verified. No paid host, personal phone, security-policy relaxation or new account is authorized by this document. Verify the required tracing capability in a suitable disposable environment, or agree on a bounded trace-enabled CI investigation, before more native runs.

## Source contracts inspected

- Android16 virtual-network explicit binding permission: https://android.googlesource.com/platform/system/netd/+/refs/heads/android16-release/server/NetworkController.cpp
- Android16 VPN establishment, UID capability ranges and seamless handover: https://android.googlesource.com/platform/frameworks/base/+/refs/heads/android16-release/services/core/java/com/android/server/connectivity/Vpn.java
- Go NewFile pollability: https://pkg.go.dev/os#NewFile

These contracts narrow hypotheses. They do not themselves prove which kernel decision caused the observed failures.
