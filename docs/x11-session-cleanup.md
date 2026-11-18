# X11 session resource and pointer cleanup

## Reproduced gap

The shared Host stopped capture when that backend supplied a closer, but never closed an owned injector. X11 capture and input had no closer at all. A remote pointer-down could therefore outlive the session, and the dedicated X connections remained open.

A fresh disposable Xvfb regression used the real XTEST injector, observed the button-down in server QueryPointer state, then failed because no owned cleanup existed. The test explicitly requires the disposable-display flag; the ordinary suite never chooses a user's desktop.

## Repair

- Host.Stop closes owned input as well as capture and retains its worker wait. Comparable shared backend objects are closed once, and repeated Stop stays idempotent.
- X11 implements idempotent Close for capture/injector connections. Close serializes with input dispatch, latches rejection of future input, releases buttons this injector recorded as left down, synchronizes and closes its dedicated X connection.
- Pointer down/up/click updates the held-button ledger only after checked XTEST requests. A failed release remains tracked for cleanup.
- Failed XTEST initialization closes the newly opened injector connection instead of leaking it.
- Capture refuses new grabs after Close. The connection can close underneath an already running grab, letting the existing capture-failure path end that work.

No capability or consent default is added. This does not implement drag support on Android, change the Wayland event model or enable a new platform backend.

## Evidence

Before repair the disposable real-X11 cleanup test failed. After repair ten fresh race repetitions pass against a newly allocated Xvfb, including a real session input stream delivering a held right-button event followed by actual Host.Stop. Server QueryPointer state confirms release, both X11 backend objects report closed, and further input is rejected. Generic Host tests check distinct/shared input/capture ownership and repeated Stop.

Fresh full Go race execution and go vet pass. The existing isolated control-plane/mesh/X11 proof also passes with chat, a 262,144-byte hash/ACK file transfer, two decoded frames and actual 1280x800 capture. Its latest frame was inspected and shows the expected red proof background. The cleanup test is now executed within that proof's own fresh display in CI. General CI is collected for the committed revision separately.

## Limits

This is a disposable X server, not a physical Xorg/desktop/compositor certificate. XTEST input state is shared by the X server, not a cryptographically isolated per-client input seat. Cleanup only knows buttons its injector left down; it cannot prove physical input provenance or restore arbitrary user intent. Already-dispatched input can finish. A blocked input command or arbitrary backend closer can delay Stop; there is no forced thread preemption or hard cleanup deadline.

Cleanup depends on a responsive X server and errors can prevent a confirmed release. Close returns those errors directly, but the existing Host.Stop API does not expose cleanup errors to its caller. An abrupt process kill/server failure bypasses cooperative cleanup. This change does not restore earlier Unicode keymap remapping or clipboard contents, and does not claim complete OS-input rollback.

Android reliability, Multies integration, separate cross-platform control consent and physical stop/recovery gates remain open. Release artifacts and the closed Android/native tracing/baseline sequences are unchanged.
