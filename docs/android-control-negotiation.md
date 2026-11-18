# Android attended control request and status

## Product boundary

Viewing admission is not input approval. The Android viewer now starts view-only and can explicitly request control from a compatible Android host. The host reviews that request in its separate local Allow/View only dialog. Opening an already allowed session's controls pauses input until the host chooses Keep or Revoke. Declining, canceling or revoking control preserves viewing on a healthy connection. A new request always needs a new local answer.

The viewer shows unavailable, available, pending, allowed, paused, declined/canceled and revoked states. It offers Cancel only while pending, and never sends touch input without both a decoded frame and the current grant. An older unsupported peer remains view-only on these new Android entry points. Generic/headless session callers retain their existing configured policy; this is not a cross-platform consent rollout.

## Wire and authorization model

- An explicit `control_v1` hello capability selects the host's dedicated `mnet-control-v1` dynamic stream. Chat and file payloads cannot supply decisions.
- Every connection has a fresh 128-bit random session nonce. Each request has a fresh 128-bit request nonce. Consumed requests cannot be replayed; the host bounds its request ledger to 256 per connection and terminates malformed/exhausted negotiation.
- Requests only create pending status. The grant source is the host-local method with the exact current session/request binding. Status revisions increase, foreign/old statuses are ignored, and a canceled/revoked request cannot be revived by a delayed grant.
- Input carries that same session/request binding. Both viewer and host check it, in addition to the existing current mesh identity/access checks and Android local accessibility gate. A canceled request racing a host answer disarms the host even if its answer was already sent.
- Local dialog handles remain one-use. Local callback handling compares the current Go snapshot, request and revision before rotating Android's dialog handle. A stale answer cannot approve a replacement request or connection.
- Control state frames use ordered non-waiting enqueue. A full outbound queue fails closed instead of blocking host pause/revoke behind a peer that stopped reading. Negotiation failure terminates the shared transport to avoid leaving the other side with a live-looking stale grant. This overload/failure exception can end viewing; ordinary Revoke does not.
- End is terminal. Late stream attachment/status cannot restore a closed gate. Accessibility interruption/unbind, rejected gesture dispatch and detected display-geometry mismatch turn off the Android gate and attempt a bound wire revoke. Geometry mismatch remains latched until hosting restarts.

## Evidence at this source milestone

Fresh full `go test -race -count=1 ./...` and `go vet ./...` pass. Focused protocol/mobile cases pass 20 fresh race repetitions, covering request without grant, exact local approval, host pause/Keep, revoke with viewing preserved, raw stale input refusal, canceled/late grants, replacement requests, unsupported negotiation, outbound backpressure, terminal End, foreign/replayed status and cancel/answer races. A real TCP loopback test exercises the mobile callback and local answer bridge with actual session framing and an injection test double. These are not native Android networking or accessibility dispatch results.

The selected real Java service/Activity framework suite passes 144 checks and the separate VPN-service suite passes 78 checks. Both use explicit Go, transport, system and accessibility fakes. Native-graphics framework renders of all seven viewer states and both host decision dialogs were inspected at 320- and 390-pixel widths. Status wraps, action labels and dialog choices fit. The dark screen area is a synthetic fixture placeholder, not received screen content. Framework rendering is not a device screenshot, system font-scale/IME/inset guarantee or OEM lifecycle proof.

A fresh arm64 Go binding builds at minimum API24. Whole-app resources, Java and minimum-API24 dex compile against that binding, and updated native instrumentation Java compiles. API-compatibility, projection/lifecycle, fixture-safety and fixed-log source/JVM guards pass. The disposable native peer source now explicitly negotiates and requests control, with one deliberate replacement request after its fixture decline; it never auto-grants. No native/emulator sequence is executed for this milestone.

## Remaining acceptance gates

This is source, loopback and simulated-framework evidence. Existing Android native DNS/bulk/recovery reliability findings, closed tracing/minimal-baseline diagnostic sequences and physical/OEM acceptance remain unchanged. Already-dispatched gestures may finish; no instant OS rollback is promised. A malicious host or compromised authorized endpoint is not made trustworthy by a status label. This is not an independent security review.

Multies enrollment/workspace roles, desktop attended-consent design, consumer file/recording approval and retention, signed iOS delivery and release/distribution decisions remain separate. No personal phone, production service, paid resource, release key or release artifact is used or changed.
