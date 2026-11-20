# Session reliability checks

Mnet is still a development build. Do not rely on file transfer for important files yet.

The proof checks changed screen content, chat, the receiver's SHA256, and the sender's verified final ACK. Received bytes alone are not a pass. The isolated X11 rig creates its own temporary directory, X display and local control-plane port. It keeps diagnostics after a failure and stops its child processes.

## TCP test observer correction (2026-11-17)

A general-CI repetition failed while awaiting a file ACK. The test observed one screen frame but left its callback feeding an undrained4-slot channel. Once that callback blocked, the bounded screen stream filled and the shared mux reader could no longer deliver unrelated file replies. A deliberate3-second delay before file transfer reproduces the timeout. Changing only the test observer to nonblocking delivery passes10race repetitions with that delay retained. Callbacks are installed at viewer creation and host cleanup is registered before transfer.

This is a fixture correction, not a new production-isolation guarantee. Real slow/blocking callbacks can still backpressure the shared reader; callback consumers must return promptly. A future scheduler/flow-control change needs separate design and regression coverage, not silent frame drops that break delta chains.

## Bounded screen callback isolation (2026-11-17)

A separate production regression now reproduces the same class with an intentionally blocked render callback:64 incoming screen frames fill the bounded inbound screen queue, preventing a subsequent chat frame from reaching its handler. The new viewer path keeps wire decoding/compositing ordered, then coalesces only completed full display images through a single-slot queue to one serial callback worker. An in-progress callback plus the latest pending display image are retained; intermediate completed display updates can be skipped. This preserves the delta chain while preventing that callback from blocking screen decoding.

The regression confirms chat delivery while the first callback is still blocked and checks the final image includes disjoint earlier/later delta updates. A second check blocks the callback through100 frames and stream completion, then requires only the initial and newest frame. Both pass20race repetitions. Restoring the old inline callback makes the chat regression fail again. Fresh uncached full-repository race tests pass. All five independent CI jobs pass at15cecdd: https://github.com/MaisySen/Mnet/actions/runs/44075184007 .

Additional callback lifecycle tests verify replacing a blocked handler sends the pending completed image to the new handler, and closing the mux before the blocked handler returns discards that pending image. Both paths pass20local race repetitions; a fresh whole-repository race run also passes. IndependentCI for these extra tests is pending.

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

## Watchdog backpressure

The separate [keepalive capacity-wait correction](session-keepalive-backpressure.md) covers a full outbound queue blocking dead-peer detection. Its scope and evidence do not close the broader loss, callback or physical-device recovery limits above.

## Absolute dirty-tile canvas coordinates

DiffTiles previously generated zero-origin rectangles even for a valid RGBA canvas or subimage with a nonzero origin. A failing-first offset-canvas test reproduced rectangles outside the actual source bounds. It now returns absolute canvas rectangles matching EncodeTiles, preserving positive/negative origins and parent strides. Empty/nil canvases and nonpositive tile sizes yield no tiles. A very large positive tile size clamps before addition, without multiplying a loop index by an unchecked step.

New tests cover positive/negative origins, unchanged/changed offset images, parent-stride subimages, changed prior bounds and maximal step size. Thirty focused delta/encode/composition race repetitions and full uncached race/vet/build pass. Both-size actual attended native view/pointer entry journeys also pass, with current received delta pixels inspected. Those actual fixtures remain ordinary zero-origin capture; nonzero-origin correctness is Go image evidence, not a claim of new physical-monitor support. No input, recording, file, capture-size or release authority changes. Fresh hosted source verification remains separate.

## Tiny offset delta round-trip fuzzing

A structured fuzz target now follows valid tiny parent-stride subimages through DiffTiles, JPEG tile encoding and composition. It varies positive/negative origins, canvas sizes, tile sizes and the changed pixel. Exactly one tile must change; destination bounds must stay exact, decoded tile pixels must match the JPEG decoder while every outside pixel remains unchanged, and neither composition nor later output edits may change the original base or its parent storage. Comparison is against decoded JPEG pixels, not lossless source colors.

Thirty focused seed/delta/composition race repetitions pass, as does a local10second mutation run with63,598executions and no failing input, then full uncached race/vet/build. Canvas/tile counts are deliberately tiny to keep per-mutation work bounded; existing image-size tests cover large limits. General CI exercises the seed corpus through ordinary tests, not a newly added hosted mutation gate. No UI, screen capture, socket, recording, input or production-account operation occurs in this target. It does not diagnose the unrelated preserved0838frame-fuzz timeout.

## Independent overnight local regression sweep

After the chooser focus/cancellation, device failure-copy, static-inspector file/termination and attended viewer SIGTERM slices, a separate full local sweep passed the100transfer userspace mesh-file regression and burst-loss recovery, whole-repository Windows amd64/Darwin arm64 cross-builds, tracked-source checks and the current Go vulnerability scan (no findings). This is not an audit, supported scale claim or native platform runtime acceptance.

The existing isolated loopback control/two-userspace-peer session proof also passed: two decoded1280x800frames, chat and262,144byte file transfer with receiver acknowledgement. It explicitly reported `deltas_smaller: false`; no new compression saving is inferred. This is the legacy engineering session path on one disposable local X display, not an attended file/recording feature, physical computers or live Multies authorization. Attended files/recording remain unavailable. The source slice's dedicated native view/pointer/Shared Desk fixtures and pending exact hosted checkpoint remain separate evidence.

The current-source integration sweep also passed both-size actual Shared Desk launcher and native source/recipient suites, both-size attended host suites and the independent attended-process cleanup suite. The process suite again confirms the important negative: directly killing the injector itself with SIGKILL leaves its synthetic left button held. Surviving-child coordinator death and a stopped coordinator's two-second lease do release their owned button, but this does not become general crash recovery. Foreign preheld input remains unowned and is not released. Both unsupported8/16bit native-control visual fixtures still refuse before window creation or daemon calls. Actual Shared Desk pending-TLS Stop pixels were inspected on equal/mixed displays; local reference checks found no missing documented image assets. Newer Apple exact-symbol acceptance remains synthetic until its own macOS run.
