# Explicit native pointer viewer

This is a separate Remote Desktop engineering entry. Existing `session view` remains view-only and never requests control. It is not Shared Desk, Multies sign-in or a released consumer app.

```text
mnet --state /path/to/viewer-state session point exact-peer-name
```

The daemon must already be up with one uniquely selected current peer. The host must run the existing attended X11 host in its local desktop console. This entry requires its attended-view protocol reply. Current peer/key and source-address verification still applies at admission and through the session; a peer label, network membership, screenshot or control request cannot grant input.

Host-local viewing approval first shows the screen, without input. In this separate pointer window:

- F8 explicitly requests the host-enforced pointer-only tier. A scope-aware attended host gets a separate default-decline pointer dialog. A pending request is not a grant. An older or generic-only host cannot silently downgrade this to keyboard-and-pointer control; this entry stays view-only and explains why.
- Only an exact current host-approved request enables pointer movement, clicks and vertical wheel steps inside the displayed image. Header/gutter/outside coordinates are refused. Fit and received-pixel 1:1/panned coordinates map back to the received frame. The host's existing injector maps capture coordinates to its native desktop.
- F9 withdraws pending, allowed or paused control, preserving viewing. Actual focus loss also withdraws it. A new F8 needs a new local host answer.
- Host decline, pause and revoke disable pointer input. Revoke leaves viewing intact. Escape, close or Ctrl-C ends the single viewing attempt. There is no reconnect or old-grant reuse.

The status header and window title show actual protocol state, including pending, allowed, declined, paused and revoked. All keyboard events stay local, including F8/F9, Escape and the existing 1/F/arrow/Home/End view controls. No text or key forwarding is implemented. The pointer entry supports atomic left/middle/right clicks and vertical wheel steps, not remote held buttons or dragging. A press sends nothing remotely; only a matching release under the same revision can send a click. Movement beyond four local pixels cancels that click. Window/frame-geometry or viewport changes, focus loss and grant changes discard a pending click. This is a narrow pointer-control slice, not complete TeamViewer-style input support.

The attended host explicitly opts into and advertises `pointer_v1` on its host-originated control stream. This entry requests `pointer-v1`; the local prompt and every accepted status remain bound to that scope, session and request. A broader or differently scoped reply is not a grant. The host input gate, not this window's UI alone, refuses keys/text, button down/up/holds, mixed key/pointer payloads, invalid buttons and wheel input other than a single vertical step. Only pointer movement and atomic left/middle/right clicks, plus those wheel steps, can reach native dispatch under this grant. Coordinate/capture checks still apply at the backend.

An explicit generic engineering control request remains generic and needs its existing local answer. Mobile/generic hosts do not advertise or accept the new tier merely by using the existing control constructor. Only the scope-aware attended host opts in. There is no automatic reconnect or broader fallback. This entry does not turn a Shared Desk approval into Remote Desktop permission, or the reverse.

## Exact queued-event authority

Native actions record the complete control snapshot, including session, request and revision. The final bind checks that same allowed snapshot atomically rather than decorating queued input with whatever grant is current later. A paused/resumed grant has a new revision, so a click queued before pause cannot be rebound after resume. Movement coalesces only within the same revision. A bounded 64-action queue ends the session on overflow rather than replaying a backlog. Local withdrawal drops queued input. The host independently rechecks its live grant at native dispatch. An OS action already dispatched before withdrawal may complete; no cancellation can recall it.

F9/focus loss uses the protocol's cancel path for an active as well as a pending request. Host input ownership is retired through the existing checked native-child cleanup. No remote button is intentionally held between local events by this entry. This does not fix X11 physical/synthetic input provenance, another application's active keyboard grab, dead-server responsiveness or a killed injector's owned-state limits. Those remain documented in the attended host and recipient-owner guides.

## Native acceptance

`python3 scripts/native-pointer-validation.py` builds the actual viewer CLI and recipient child and runs an attended host fixture on two disposable X servers. Synthetic local API identity is explicitly fixture evidence, not a live Multies integration. Both 1024x768 and 2048x1536 host sizes are exercised. The driver:

- Answers the real host view dialog, waits for actual host pixels and verifies local clicks/keys before a control request do not reach the target.
- Explicitly requests and answers the real host pointer-only control dialog, then observes two native target left-button press events and one native wheel event. The test also attempts a local keyboard event, which must not reach the target.
- Withdraws locally, gets a host decline, obtains a new grant, loses actual viewer focus, obtains another new grant and invokes actual host Revoke. Clicks while declined/revoked must not add target input. Exactly three grants create native input owners.
- Joins session teardown and native cleanup, drains target events through a same-connection marker, and requires exact click/scroll/key counts and no held native left button. Viewing must survive control withdrawal/revoke until explicit End.

The first driver tried input before any actual screen image, and its guessed keyboard traversal did not select Revoke. Actual pixels exposed both assumptions. It now waits for real image content and native target events, and uses the inspected Revoke button band with actual revoked-state acceptance. An early end-of-test nonblocking event poll could miss queued X events; the target event drain is now explicitly joined instead of treating an empty instantaneous poll as a complete event history. These driver corrections do not relax grant, input-count or cleanup assertions. Visual inspection also caught a title/pixel ordering gap: the window title could report the new grant state while its header still showed the previous state during redraw. The renderer now publishes the corresponding completed pixels before updating the title; final allowed/revoked pixels were re-inspected. Unit tests cover fit/letterbox/1:1 mapping, exact click revisions, unsupported drag refusal and local release queue clearing. Protocol tests reject pre-pause input after resume, broader raw input sent around the viewer gate, scope-confused replies, unknown scopes and generic-only/older hosts. The explicit generic path remains covered separately. Local-answer tests verify scope reaches the prompt, not just the wire.

A larger-size native run found a real in-flight retirement bug: a wheel event raced local F9 withdrawal; the child reported retired authority and then confirmed clean release, but the coordinator treated retirement as device failure and ended viewing. Instrumented native repetition reproduced that exact result. A deterministic failing-first regression now distinguishes only the checked backend's typed retirement result after authority actually ended. It joins cleanup before preserving viewing and still reports that the command was interrupted, not successful. Unrelated dispatch failures, a retirement error while authority is still live, and unconfirmed cleanup remain terminal. A protocol-only subprocess fixture deliberately withholds its ACK until cancellation to make this ordering repeatable. Repeated native acceptance and race tests follow the correction; no failure is hidden by weakening the input-count or view-preservation assertions.

The unchanged view-only native journey is run again to check its existing no-injector invariant and local view controls. Source gates include full uncached race tests, vet/build and Windows/Darwin viewer cross-builds. Fresh hosted validation is separate from these local results. These are real native processes/pixels on one machine, not physical two-computer, secure-desktop, Wayland, Windows/macOS input or consumer-readiness acceptance. Files, recording and clipboard remain disabled in attended mode.

## Actual local pixels

![Actual pointer window after host approval and receipt of the host desktop](ui/desktop-viewer/pointer-allowed.png)

![Actual host pointer-only approval dialog](ui/desktop-viewer/pointer-host-approval.png)

![Actual revoked state while viewing remains open](ui/desktop-viewer/pointer-revoked.png)
