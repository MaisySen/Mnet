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
- Preventing an avoidable local relay overflow and changing proof-stack recovery do not establish correct recovery under every loss profile. There is limited margin under the 30-second ACK deadline in these artificial burst profiles; physical-device, broader OS TCP profiles and long sessions remain outstanding. The bounded Linux kernel/OS TCP evidence below is separate.
- Temporary dependency-timer experiments did not pass repeated verification and were not retained. There is no maintained dependency patch or proven timer-root-cause claim.

Keep burst-loss recovery open until a repeatable baseline failure, an explained change, repeated passing fault tests and independent CI support closing it. Passing ordinary reruns is not a diagnosis.

## Linux kernel and OS TCP evidence

The disposable `scripts/kernel-vpn-validation.py` CI fixture uses real kernel TUNs and ordinary OS TCP, not the proof-only gVisor adapter. It checks both peers' direct UDP endpoints, then stops the control/relay process before transferring 3 x 4MiB normally and 3 x 4MiB with 1% netem loss plus 20ms delay. Receiver SHA256, sender ACK, nonzero actual drop counters, and Down interface/mesh-route removal are required. [c4a1bcd CI](https://github.com/MaisySen/Mnet/actions/runs/43760903768) passed and observed 132 dropped underlay packets.

That stronger test initially failed before direct path selection. A bounded direct retry was suppressed by healthy relay handshakes; own-overlay-address discovery also offered an unusable underlay candidate. Both source fixes were needed before this fixture passed. This does not establish arbitrary NAT traversal, exit routing, Windows/macOS VPN operation or production soak reliability.

A later repeated kernel run (`875cd1a`, [CI](https://github.com/MaisySen/Mnet/actions/runs/43764852166)) still remained on relay despite valid candidates before the ACL extension ran. This is an open repeatability limit, not invalidation of the earlier passing transfers. The next source change uses an explicit authenticated-handshake probe window: retry candidates at a bounded interval, do not count healthy relay roaming as direct success, fall back after 30 seconds and wait for the 90-second retry cadence. The first independent [7a72378 run](https://github.com/MaisySen/Mnet/actions/runs/43765437995) passed direct selection, saved-state cycles, route-only ACL withdrawal/restoration and the control-off transfer/loss profiles (119 drops). Three fresh fixture runs are required next before closing the intermittent-convergence item.
