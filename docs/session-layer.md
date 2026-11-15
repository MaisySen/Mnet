# Mnet session layer

Remote-desktop-style sessions between two Mnet peers, built on the mesh
that already exists. This is the layer TeamViewer's product adds on top
of its networking: screen, input, chat and file transfer as typed
streams over one connection.

## What the mesh already gives us

The TeamViewer teardown mapped their shape onto ours:

- Both-sides-outbound NAT traversal: done (STUN endpoint discovery,
  DERP-style relay fallback).
- End-to-end encryption: done (WireGuard between every peer pair).
  TeamViewer needed a second TLS layer inside sessions because their
  routers see session traffic. Our relay only ever sees WireGuard
  ciphertext, so the session layer runs over the mesh WITHOUT a second
  crypto stack: one cipher suite, no negotiation, no downgrade surface.
  The 2014-style "legacy encryption request" anti-pattern is designed
  out, not patched out.
- Identity and authorization: control plane node records, key expiry
  and per-network ACL policies already decide which pairs may talk.

## Design

One TCP connection per session, dialed to the host's mesh IP on port
5909. Typed streams are multiplexed over it with priority classes, the
same early design decision TeamViewer made (their StreamType enum).

Priorities (0 is most urgent):

| Class | Use |
| --- | --- |
| 0 | Input events, session control: interactivity lives here |
| 1 | Screen frames |
| 2 | Chat, clipboard, capability messages |
| 3 | File transfer bulk data |

The writer drains higher classes first so a big file pull never makes
the pointer lag.

## Streams

| ID | Type | Direction | Payload |
| --- | --- | --- | --- |
| 0 | control | both | JSON: hello, capabilities, accept/refuse, end |
| 1 | screen | host to viewer | frame messages (codec, rects) |
| 2 | input | viewer to host | JSON pointer/key events |
| 3 | chat | both | UTF-8 text |
| 4+ | file | both | offer, accept, chunks, acks |

More stream kinds (audio, shell, port forwarding) slot in as new IDs
without protocol changes.

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

JPEG frames (quality ~70), capped at 1280 px on the long edge, 5 to 10
fps depending on change. v1 sends full frames when anything changed;
dirty-rect deltas are a compatible later optimization (frame message
carries a rect list, so viewers can handle both from day one).

Host capture: Android MediaProjection; Linux X11 shared memory.

## Input v1

JSON events: {"kind":"pointer","x":..,"y":..,"button":..,"action":..}
and {"kind":"key","text":..} in host coordinates. Host injection:
Android accessibility gestures and key events; Linux XTEST.

## Chat v1

UTF-8 text, one DATA message per line.

## File v1

Offer (name, size, sha256), accept with resume offset, 256 KiB chunks,
final ack with the digest. Priority class 3.

## Security model

The mesh authenticates and encrypts the path; the host decides consent:
an accept prompt (app) or an allowlist (headless daemon). Every session
writes one audit line on both sides with peer name and duration. No
passwords on top of the mesh.

## Pieces

- internal/session: framing, mux, stream types, handshake, host and
  viewer logic, file transfer (done, race-tested)
- mnetd -session-host -session-auto-accept: desktop host on Linux.
  X11 sessions capture with XGetImage and inject with XTEST. Wayland
  sessions capture with grim (wlroots compositors) and inject with
  ydotool (evdev level, compositor agnostic). GNOME and other
  compositors without wlr-screencopy need the portal-native D-Bus +
  PipeWire backend (later).
- mnet session connect / mnet session send: terminal viewer with
  chat, frame saving, file push (done)
- Android: HostSessionService (MediaProjection capture, consent
  dialog or auto-accept), MnetAccessibilityService (gesture
  injection while hosting), ViewerActivity (frames, touch input,
  chat) (done; key text injection intentionally out of v1)
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
