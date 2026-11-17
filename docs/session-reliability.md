# Session reliability checks

Mnet is still a development build. Do not rely on file transfer for important files yet.

The proof checks changed screen content, chat, the receiver's SHA256, and the sender's verified final ACK. Received bytes alone are not a pass. The isolated X11 rig creates its own temporary directory, X display and local control-plane port. It keeps diagnostics after a failure and stops its child processes.

## TCP test observer correction (2026-11-17)

A general-CI repetition failed while awaiting a file ACK. The test observed one screen frame but left its callback feeding an undrained4-slot channel. Once that callback blocked, the bounded screen stream filled and the shared mux reader could no longer deliver unrelated file replies. A deliberate3-second delay before file transfer reproduces the timeout. Changing only the test observer to nonblocking delivery passes10race repetitions with that delay retained. Callbacks are installed at viewer creation and host cleanup is registered before transfer.

This is a fixture correction, not a new production-isolation guarantee. Real slow/blocking callbacks can still backpressure the shared reader; callback consumers must return promptly. A future scheduler/flow-control change needs separate design and regression coverage, not silent frame drops that break delta chains.

## Bounded screen callback isolation (2026-11-17)

A separate production regression now reproduces the same class with an intentionally blocked render callback:64 incoming screen frames fill the bounded inbound screen queue, preventing a subsequent chat frame from reaching its handler. The new viewer path keeps wire decoding/compositing ordered, then coalesces only completed full display images through a single-slot queue to one serial callback worker. An in-progress callback plus the latest pending display image are retained; intermediate completed display updates can be skipped. This preserves the delta chain while preventing that callback from blocking screen decoding.

The regression confirms chat delivery while the first callback is still blocked and checks the final image includes disjoint earlier/later delta updates. A second check blocks the callback through100 frames and stream completion, then requires only the initial and newest frame. Both pass20race repetitions. Restoring the old inline callback makes the chat regression fail again. Fresh uncached full-repository race tests pass. IndependentCI remains pending for this source change.

This is limited to viewer screen callbacks. Expensive wire decoding, other blocked callbacks and general inbound stream flow control can still backpressure the shared mux. A permanently blocked user callback cannot be interrupted by closing the mux and retains its worker until it returns. Recording on the host is unchanged. This source change is not in the preserved published code15 bundle or the live service.

## Parser fuzz coverage (2026-11-17)

New test targets exercise untrusted session-frame and public STUN-reply bytes. Frame decoding must remain within the1MiB cap and successful decode/re-encode must preserve the exact consumed wire prefix. Accepted STUN replies must be complete and reject a different transaction ID. Local bounded10-second two-worker runs passed321,212session inputs and564,035STUN inputs without a failure; this is sampling, not a parser-security certification. Fresh uncached whole-repository race tests and the current vulnerability scan pass. Standard free publicCI now runs each fuzz target for10seconds with two workers; no private fixture state is uploaded. The preserved owner AAB and live deployment remain unchanged.

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

A later repeated kernel run (`875cd1a`, [CI](https://github.com/MaisySen/Mnet/actions/runs/43764852166)) still remained on relay despite valid candidates before the ACL extension ran. At that point this was an open repeatability limit, not invalidation of the earlier passing transfers. The next source change uses an explicit authenticated-handshake probe window: retry candidates at a bounded interval, do not count healthy relay roaming as direct success, fall back after 30 seconds and wait for the 90-second retry cadence. The first independent [7a72378 run](https://github.com/MaisySen/Mnet/actions/runs/43765437995) passed direct selection, saved-state cycles, route-only ACL withdrawal/restoration and the control-off transfer/loss profiles (119 drops). Three fresh full fixtures then passed in [2d23b6a CI](https://github.com/MaisySen/Mnet/actions/runs/43765976816), observing 135, 149 and 169 actual underlay drops. Each included direct convergence, two saved lifecycle cycles, ACL route withdrawal/restoration, control-off TCP profiles and owned-route cleanup. This closes that bounded reproduction, not arbitrary NAT/public-internet/soak reliability.

The actual admin-expiry extension passed three fresh [0d56bc7 fixtures](https://github.com/MaisySen/Mnet/actions/runs/43766698566): delete fixture enrollment keys, expire one node, observe peer/subnet-route removal and failed reachability, then admin reapproval restores direct traffic. Subsequent OS TCP loss profiles observed 154/152/161 drops. This did not initially assert the displayed local status after reapproval. A separate local regression reproduced a stale waiting-approval status/link; its correction adds that assertion before declaring the reapproved fixture ready. The correction and stronger status/link assertion passed all five jobs and three fresh kernel fixtures in [e6ab28d CI](https://github.com/MaisySen/Mnet/actions/runs/43767516777), including 136/154/164 observed loss-profile drops.

A separate LAN namespace (no mesh return route) and control-off LAN OS TCP verify owned, destination-scoped subnet NAT. All five jobs and three fresh full fixtures passed in [6735500 CI](https://github.com/MaisySen/Mnet/actions/runs/43770289687), with LAN TCP 3 x 4MiB hash/ACK in each, lifecycle/ACL/expiry/reapproval/cleanup and later mesh loss-profile drops 154/120/114. It requires forwarding to be explicitly enabled by the disposable fixture; the daemon no longer changes global forwarding.
