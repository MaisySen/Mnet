# Mnet session layer

Remote-desktop-style sessions between two Mnet peers, built on the mesh
that already exists. This is the layer TeamViewer's product adds on top
of its networking: screen, input, chat and file transfer as typed
streams over one connection.

## Readiness and security boundary

Implementation is not physical-platform certification. [STATUS](STATUS.md), [native validation](native-platform-validation.md) and [reliability](session-reliability.md) define the checks and remaining gaps.

WireGuard encrypts the mesh path, including relay traffic. The session protocol adds no second encryption layer. That does not remove its parser, consent, file and capture/input attack surfaces; independent review remains open. Pair-level ACLs and source-IP/name binding authorize the connection, not arbitrary application-port policy.

## Design

One TCP connection per session, dialed to the host's mesh IP on port
5909. Typed streams are multiplexed over it with priority classes, with bounded queues and weighted scheduling.

Priorities (0 is most urgent):

| Class | Use |
| --- | --- |
| 0 | Input events, session control: interactivity lives here |
| 1 | Screen frames |
| 2 | Chat, clipboard, capability messages |
| 3 | File transfer bulk data |

Weighted scheduling favors higher classes while giving bulk traffic progress. This is not a latency guarantee under every network or workload.

## Streams

| ID | Type | Direction | Payload |
| --- | --- | --- | --- |
| 0 | control | both | JSON: hello, capabilities, accept/refuse, end |
| 1 | screen | host to viewer | frame messages (codec, rects) |
| 2 | input | viewer to host | JSON pointer/key events |
| 3 | chat | both | UTF-8 text |
| 4+ | file | both | offer, accept, chunks, acks |

Audio, shell and port forwarding are not implemented session capabilities. Adding them requires protocol, authorization and compatibility review.

## Wire format

Handshake on the control stream, then framed messages:

    header (12 bytes, big endian):
      u32 stream id
      u8  message type: 1=OPEN 2=DATA 3=FIN 4=RST 5=PING 6=PONG
      u8  priority class (0-3)
      u16 flags (reserved, zero)
      u32 payload length (max 1 MiB)
    payload

OPEN carries a JSON object describing the stream (codec, dimensions,
file name and size, ...). DATA on screen/input/chat/file carries the
payloads above. FIN half-closes, RST aborts. PING/PONG keep the path
warm and measure latency.

## Screen v1

JPEG frames and 64x64 changed-tile deltas with periodic keyframes are implemented. Backend capture size, frame rate and quality vary; the desktop host typically requests 8fps and can adjust supported capture quality with observed RTT. Deltas are composited onto a previous frame. These are implementation choices, not measured physical-platform frame-rate guarantees.

Capture backends include Android MediaProjection and Linux X11/Wayland paths. X11 proof runs under Xvfb; real GNOME portal/PipeWire and physical mobile capture remain open.

## Input v1

JSON events: {"kind":"pointer","x":..,"y":..,"button":..,"action":..}
and {"kind":"key","text":..} in host coordinates. Host injection:
Android accessibility gestures and key events; Linux XTEST.

## Chat v1

UTF-8 text, one DATA message per line.

## File v1

Offer (name, size, sha256), acceptance, 256KiB chunks and final digest ACK. Priority class 3. Receiver acceptance starts from offset zero; resumable transfers are not implemented.

## Security model

The mesh authenticates and encrypts the path; the host decides consent:
an accept prompt/app policy or explicit auto-accept on the headless daemon, optionally narrowed by an allowlist. The desktop host records accept/refuse/end in its state-directory audit log. A symmetric audit on every viewer/platform is not guaranteed. No
passwords on top of the mesh.

## Pieces

- internal/session: framing, mux, stream types, handshake, host and
  viewer logic, file transfer (implemented with race-tested local regressions)
- mnetd -session-host -session-auto-accept: desktop host on Linux.
  X11 sessions capture with XGetImage and inject with XTEST. Wayland
  sessions capture with grim on wlroots compositors, or through the
  XDG ScreenCast portal (D-Bus negotiation in pure Go, frames pumped
  out of PipeWire by a gst-launch helper) on compositors without
  wlr-screencopy, GNOME above all; MNET_CAPTURE=portal forces the
  portal path. Input injects with ydotool (evdev level, compositor
  agnostic) on every Wayland backend.
- mnet session connect / mnet session send: terminal viewer with
  chat, frame saving, file push (implemented; see platform validation)
- Android: HostSessionService (MediaProjection capture, consent
  dialog or auto-accept), MnetAccessibilityService (gesture
  injection while hosting), ViewerActivity (frames, touch input,
  chat) (source implemented; key text injection intentionally out of v1, physical validation pending)
- iOS: viewer only; hosting needs ReplayKit and is parked with the
  Apple account work

Text injection on Linux hosts is selectable with MNET_TEXT_MODE:
auto (default) types per rune via keysyms on X11 (remapping a spare
keycode for characters outside the current layout, so Unicode works)
and via ydotool type on Wayland. MNET_TEXT_MODE=clipboard instead
sets the clipboard (xclip/xsel on X11, wl-copy on Wayland) and
injects Ctrl+V, which pastes any text the tool accepts but clobbers
the user's clipboard. Frames stream as dirty-tile deltas (64x64 JPEG
tiles) when only part of the screen changes, with a full keyframe
every 100 frames; viewers composite tiles onto the base frame.


## File reception policy and backpressure

Incoming files are limited to 1GiB each by default (FileReceiver.MaxBytes
can set a different limit). A destination that already exists is refused;
rename the source before resending. Path separators, traversal, invalid
sizes and malformed supplied hashes are refused. Partial transfers stay
in private temporary files and are removed on failure. Verified complete
files publish at mode 0600 without replacing an existing destination.
The receive filesystem must support hard links for atomic publication;
an unsupported filesystem fails the transfer instead of falling back to
an unsafe overwrite. This is not a resumable-transfer implementation.

Outbound frames have a 16MiB queue cap, excluding one in-flight write.
Senders wait for capacity and wake on close. Weighted round-robin drains
input, screen, chat and file classes so a busy screen cannot indefinitely
starve file completion. Each side also caps active streams at 256, including well-known channels;
excess incoming streams end the session and excess local opens fail.
Completed file-transfer entries are released, and a reset is distinct
from a successful FIN. These limits bound selected queues and tables,
not all per-session memory or process-wide concurrent connections.


## Screen decode recovery and Android input limits

Go viewers and recorders reject invalid full JPEGs and failed deltas, discard the composition base and wait for a valid keyframe. Tile rectangles must fit the base and match decoded JPEG header dimensions. These checks bound rectangle work, not all image-decoder or per-session memory. Closing a recorder clears retained base pixels; late frames are ignored.

Android sends one click on a completed stationary release after a decoded frame is available. Drag/multitouch/held-pointer control is not implemented. Host-side validation accepts completed click/fixed scroll events with finite integer coordinates inside capture bounds, preserving Go omitted-zero coordinates. It rejects down/up rather than reinterpreting each as a click. A host display width/height/rotation mismatch disables input until capture is restarted. This is a fail-closed limit, not dynamic rotation support.

See [current framework coverage](../android/framework-tests/README.md) for source versus fake endpoints and simulated API limits. No physical capture/input or JNI runtime proof is implied.

## JPEG pixel-allocation policy (48fcffc all-five CI verified)

Go viewers and recorders check JPEG headers before full-image decoding. Each dimension must be positive and at most8192, and their product must be at most16,777,216 pixels. The division-based check avoids integer overflow.4K3840x2160 fits;8K7680x4320 does not. Tile headers and the composition base use the same dimension policy. An oversized keyframe is rejected and invalidates its old delta chain.

A tiny JPEG fixture with rewritten65535x65535 header dimensions tests refusal without allocating that pixel image. Boundary/overflow tests, viewer/recorder rejection and existing recovery tests pass30 race-enabled repetitions. A fresh full-repository race run and all-five [48fcffc CI](https://github.com/MaisySen/Mnet/actions/runs/43937696893) pass. This bounds a decoded image's pixel dimensions, not codec CPU, all decoder overhead, concurrent sessions or aggregate heap; compositing/callbacks can keep multiple images. No current-source Android binding rebuild, APK/AAB publication, device/capture/UI/JNI proof or live deployment for this change.

## Aggregate delta tile work (13fa2e2 retry verified)

A delta is limited to4096 tiles and at most the base frame's pixel count summed across all tile rectangles, before pixel decoding or composition. Regular nonoverlapping64x64 updates fit the budget; repeated overlapping rectangles that exceed one frame's work are refused. This adds a per-delta count/pixel-work limit to the existing individual geometry/JPEG checks, not a general CPU or total memory guarantee.

The aggregate-overlap regression failed before the guard. Tests accept exact four-quadrant coverage and reject excess aggregate pixels or tile count. Targeted frame/recorder/composition race checks pass30 repeats; a fresh full-repository race run passes. [13fa2e2 CI attempt2](https://github.com/MaisySen/Mnet/actions/runs/43939827797) passed after the first dependency-download failure. Synthetic Go codec data only, not physical capture/UI/JNI proof. No current-source Android AAR, release APK/AAB or live deployment produced for this change.

## Session truncated-write error detection (4bce9b8 all-five CI verified)

Session framing now returns io.ErrShortWrite if a writer reports fewer header or payload bytes without an error. It does not claim success or continue with a payload after an incomplete header; existing transport errors remain unchanged. The mux's existing write-error path closes the session. This protects framing at its io.Writer boundary, including custom adapters that violate the usual short-write error contract; it is not automatic retry or a reproduced kernel TCP failure.

A short-header regression failed before the fix. Header/payload truncation and transport-error preservation tests pass100 race-enabled repetitions; fresh full-repository race tests pass. All-five [4bce9b8 CI](https://github.com/MaisySen/Mnet/actions/runs/43946082516) passes. Synthetic writer evidence only, no live network/device/runtime claim. No new bindings, release APK/AAB or deployment. Separately, [905a401 all-five CI](https://github.com/MaisySen/Mnet/actions/runs/43944038163) confirms Maven cache restoration and64 passing framework cases.

## Session Dial transport ownership (2e87ce6 all-five CI verified)

Dial now explicitly owns its supplied connection: a failed hello write, reply read/parse, unexpected frame/version or refusal closes it. Success transfers ownership to the returned mux, which closes it later. Callers still supply handshake deadlines; this does not add a timeout or change consent rules. Existing caller-side error cleanup may close an already-closed connection, which net.Conn permits.

Counted net.Pipe regressions failed before the change for bad reply/frame/version/refusal and failed hello write. Error closure and successful mux ownership pass30 race repetitions; fresh full-repository race tests pass. All-five [2e87ce6 CI](https://github.com/MaisySen/Mnet/actions/runs/43948329722) passes. These are local synthetic transport tests, not a reproduced live descriptor leak or device/OS-network proof. No fresh bindings, release APK/AAB or deployment.

## CLI session handshake deadline (4f93413 all-five CI verified)

CLI session connect/reconnect and file-send now allow70 seconds for hello/host consent after TCP connects, rather than waiting indefinitely for a silent host. The existing TCP connect limit remains10 seconds. A successful handshake clears its deadline before active use; a failure closes the connection through Dial ownership. Expiry can terminate a delayed consent response and requires another attempt; it does not change authorization policy.

Short-budget silent-host and successful deadline-clear net.Pipe tests pass30 race repetitions. Fresh full-repository race tests pass. These use an accelerated fixture timeout, not an elapsed70-second real host/device test. All-five [4f93413 CI](https://github.com/MaisySen/Mnet/actions/runs/43950358961) passes. No mobile source, bindings, release artifacts or live deployment changed.

## Headless desktop host hello budget (d55f4d3 all-five CI verified)

The desktop session host bounds accepted-socket hello/reply I/O to15 seconds, then clears the deadline before starting capture. A silent peer no longer occupies the serial accept/hello loop indefinitely. Headless authorization remains immediate and unchanged; mobile's separate consent allowance is unchanged. This is a deadline, not a per-peer admission/connection-rate limit.

An accelerated40ms real OS-loopback test accepts a silent connection, verifies timeout, accepts a subsequent approved viewer and measures a successful keepalive round trip after the original handshake deadline.30 race repetitions and a fresh full-repository race run pass. No physical host, public-internet timing,15-second elapsed run, capture/input or device proof. All-five [d55f4d3 CI](https://github.com/MaisySen/Mnet/actions/runs/43952410300) passes. No bindings, release artifacts or live deployment changed.

## File receiver idle cleanup (new source, CI pending)

Incoming file reception now allows60 seconds between data/FIN frames by default. A silent incomplete stream returns, closes/removes its private temporary file and releases its stream entry without closing unrelated session channels. FileReceiver.IdleTimeout can select a different positive budget; nonpositive uses the default. Active progress restarts the per-receive wait, so this is not a total-transfer deadline or resumable-transfer feature. A sender paused past the budget must retry the file.

A short40ms net.Pipe test sends a partial file, waits for idle cleanup and checks empty destination directory, stream release and a still-open mux. Idle/safe-file/reset tests pass30 race repetitions; a fresh full-repository race run passes. No elapsed60-second real network test, device/runtime, new bindings, release APK/AAB or live deployment proof.
