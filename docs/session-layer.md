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

A screen-production failure now ends the host session transport instead of leaving a keepalive-connected frozen screen. That also ends its chat/file traffic; reconnect requires capture to work again. Recording-only output failure remains separate and does not end hosting. Full/delta/keyframe-fallback failure coverage and all-five CI pass at 0396795. A mobile owner-level callback/listener reuse test passes locally, not physical Android capture proof.

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

## Image limits and viewer lifecycle

JPEG headers are checked before full decoding: each side is at most 8,192 pixels and the product at most 16,777,216 pixels. A 3,840 x 2,160 image fits; 7,680 x 4,320 does not. A delta has at most 4,096 tiles, and summed tile rectangle pixels cannot exceed the base frame's pixel count. These are per-image/per-delta checks, not codec CPU, aggregate heap or concurrent-session limits.

Malformed viewer screen JSON also invalidates the old delta base. The viewer waits for a valid keyframe instead of composing subsequent deltas on stale pixels. When the screen loop exits after FIN/reset/mux closure, it releases its decoded base. Callback-retained copies and callbacks that block are outside that cleanup. No sequence-gap detector is added.

## Transport ownership and handshake budgets

Dial takes ownership of its connection: failures close it, success transfers it to the mux. Callers set handshake deadlines. CLI connect/reconnect and file-send allow 70 seconds for hello/consent after a 10-second TCP connection budget. The desktop headless host allows 15 seconds for accepted hello/reply I/O. Success clears these deadlines before active use. Mobile consent uses its separate existing allowance. These are timeouts, not admission/rate limits or changes to authorization.

Framing rejects silent short header/payload writes with io.ErrShortWrite. Unsupported v1 types, priorities outside 0-3 and nonzero reserved bits reject before payload allocation/read. All six defined types remain supported. Future extensions need explicit format/compatibility work; this is not complete message-body validation.

## File idle cleanup and stream release

The default incoming file idle budget is 60 seconds. FileReceiver.IdleTimeout can set a positive override. Only successfully written nonempty bytes restart it; empty/unrelated frames cannot retain a partial transfer. Actual progress may continue beyond one budget, so this is not a total-transfer cap. FIN can complete an empty offered file.

Idle expiry attempts a best-effort stream reset, then closes/removes the private partial file and releases its stream. If outbound capacity is full, notification is skipped rather than waiting for the peer to drain. A reachable sender can observe ErrStreamReset, but reset delivery is not guaranteed and already queued outgoing chunks are not canceled. A paused sender must retry, not resume. Filesystem/scheduler timing has no hard wall-clock guarantee.

Local stream release interrupts an in-flight read-loop DATA/reset delivery into that stream's full queue. The loop can then process unrelated streams. The read loop alone closes frame queues. Ordinary head-of-line blocking still exists for an unreleased slow consumer.

## Session recording

Recordings use concatenated JPEG frames. Keyframes are validated; deltas are composited and re-encoded. Broken chains wait for a valid keyframe. NewRecorder exclusively creates a mode-0600 destination. Existing files/final symlinks are refused, never truncated. The daemon's viewer/timestamp paths can collide within one second; recording then reports unavailable while hosting continues. Parent directories must be trusted.

The first output error stops recording, closes the writer and releases pixels; completed frames remain counted. Close retains the first write error, or a close error when no earlier write failed, across repeated calls. The daemon logs recording incomplete at session end. It keeps partial bytes and does not retry, repair, fsync or promise durability. A file or nonzero frame count alone does not establish a complete recording. Late callbacks after close/failure are ignored.

## Validation and version boundaries

The behavior above is newer source, not a claim about unchanged v0.8.2-test.1 downloads or the last dated hosted deployment. [STATUS](STATUS.md) owns current source/build/release/deployment state. [Native validation](native-platform-validation.md) owns exact regression history, CI URLs and fixture limits, so this guide does not repeat stale per-commit pending/build notes.

At 0396795, all five CI jobs passed. Fresh local full gates and four-ABI Android binding/resource/Java/min-api-24 dex checks passed, plus 64 framework simulation cases. This does not execute JNI, physical capture/input, Android UI pixels, install or boot. No signed APK/AAB or release/deployment followed these checks.
