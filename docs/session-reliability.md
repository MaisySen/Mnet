# Session reliability checks

Mnet is still a development build. Do not rely on file transfer for important files yet.

The proof checks changed screen content, chat, the receiver's SHA256, and the sender's verified final ACK. Received bytes alone are not a pass. The isolated X11 rig creates its own temporary directory, X display and local control-plane port. It keeps diagnostics after a failure and stops its child processes.

## Commands

```sh
sh scripts/local-gates.sh
sh scripts/session-validation.sh
MNET_MESH_STRESS=1 go test -race -count=1 -timeout=2m ./cmd/mnetd -run '^TestMeshFileStress$'
```

The mesh stress test transfers 100 files over actual userspace WireGuard nodes, a local control plane and relay. It does not require X11. It runs in local gates and CI. These are local integration checks, not physical-phone, desktop-installation, public-network or fleet validation.

## Fault injection

```sh
# Drop one large encrypted data datagram, then allow traffic again.
MNET_MESH_STRESS=1 MNET_MESH_LOSS=single go test -race -v -count=1 -timeout=2m ./cmd/mnetd -run '^TestMeshFileStress$'

# Drop large encrypted data datagrams during one 20ms window.
# Compare proof-only classic SACK (default) with RACK (MNET_PROOF_RACK=1).
MNET_MESH_STRESS=1 MNET_MESH_LOSS=burst MNET_PACKET_TRACE=1 MNET_SESSION_TRACE=1 go test -v -count=1 -timeout=2m ./cmd/mnetd -run '^TestMeshFileStress$'
```

The proxy reads only the WireGuard message type and packet size for injection. Packet tracing records TCP sequence/ACK numbers, flags, sizes and times at the test TUN boundary. It does not retain packet contents or addresses. Session tracing retains a bounded 512-event metadata ring per session and writes it on close; it does not record application payloads, file names or credentials. Both traces are off in normal operation.

## Evidence and limits

- The original relay queue immediately dropped packets when its 64 slots filled. The mesh stress test failed at the first transfer, with measured relay drops. A larger-queue experiment passed, but was not retained as the fix.
- The queue remains bounded at 64 packets. A full queue now gets at most 100ms to drain. Tests cover brief pressure, a permanently slow destination, detach, unrelated-destination progress, network scope and authorization revalidation before queued traffic is written.
- Fresh repeated mesh tests pass after that prevention change. Single-datagram loss also recovers in the tested profile.
- The pinned userspace proof stack with default RACK can exceed the 30-second final-ACK deadline after a short loss burst. The same adapter with classic SACK passed repeated 20ms and 50ms profiles. `MNET_PROOF_RACK=1` preserves the failing comparison. `MNET_LOSS_MS=1..1000` varies the loss window.
- The proof now uses a small attributed `internal/proofnet` bridge and the exported TCPRecovery setting, not a dependency fork. This configuration applies only to the diagnostic proof. Desktop and mobile sessions use OS TCP on their VPN interfaces. No production OS-TCP failure or fix is established by this experiment.
- Preventing an avoidable local relay overflow and changing proof-stack recovery do not establish correct recovery under every loss profile. There is limited margin under the 30-second ACK deadline in these artificial burst profiles; physical-device and OS TCP checks remain outstanding.
- Temporary dependency-timer experiments did not pass repeated verification and were not retained. There is no maintained dependency patch or proven timer-root-cause claim.

Keep burst-loss recovery open until a repeatable baseline failure, an explained change, repeated passing fault tests and independent CI support closing it. Passing ordinary reruns is not a diagnosis.
