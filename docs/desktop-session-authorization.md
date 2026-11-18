# Desktop host current-authorization enforcement

## Reproduced production gap

The headless desktop listener checked the first display-status peer with a matching name/IP. It did not require a unique currently approved/unexpired full-key match. After admission the shared Host engine did not recheck authorization before capture, recording, input, chat or incoming file handling.

A real mux/capture regression supplied a changing known-authorization callback to the shared host. Before implementation the admitted connection remained alive after denial; the test failed. The new option was initially a no-op, then implemented. This isolates the active-host defect without pretending to execute a physical desktop capture backend.

## Source changes

The headless listener now uses the daemon's private `SessionPeers` evidence instead of display Status. The canonical name, unique TCP source and canonical full key are pinned and rechecked after admission and during active hosting. Explicit headless auto-accept and optional allow-list requirements remain unchanged.

The canonical matcher previously used by mobile now lives unchanged beside the daemon's private session-peer snapshot; both callers use the same exact rules. Moving it does not add a new identity source, trust grant or Multies verifier.

The shared Host has an optional authorization callback. Once it observes denial, that Host cannot revive. It closes the mux without waiting on its own workers and checks:

- before and after full/delta capture, including before a keyframe fallback;
- before recording and screen publication;
- before input/chat dispatch and host chat sending;
- before incoming-file storage, each data write, verified publication and completion notification.

A separate local snapshot watcher closes idle/backpressured sessions and sessions with a blocked capture callback. It uses an 8 Hz local check, not external polling, a freshness lease or a hard wall-clock guarantee. Existing Host users without an authorization callback retain their admission model; the headless desktop production caller supplies one. The mobile caller retains its own guard.

## Verification

Fresh race tests verify active denial closure, closure while capture is blocked, preserved capture Close, terminal invalidation after map restoration, suppression of queued input/chat, full/delta/fallback capture denial before recording, and incoming-file temporary cleanup without publication/notification after denial. Desktop identity fixtures cover missing keys and duplicate names/source addresses. Existing mobile identity fixtures continue exercising the moved matcher.

Fresh full Go race execution and go vet pass locally. General CI is collected for the committed revision separately. This is not physical X11/Wayland capture/input, OEM Android, public-network or scale certification.

## Limits

Only currently known daemon authorization can be enforced. Unknown offline administrative changes remain unknown. The callback checks do not create a Multies person/workspace/session contract, a separate desktop control-consent journey or per-file consent. Headless auto-accept remains combined admission and is not the target consumer approval design.

A platform action, recording write or filesystem operation already dispatched may finish. A capture callback cannot be forcibly interrupted; transport closure and capture cleanup are distinct. Initial platform capturer construction precedes the shared Host watcher, so a constructor that blocks cannot be canceled by this watcher; authorization is rechecked when Host setup begins. Existing file writes use private temporary data and cleanup on return, but this is not a transaction with authorization: a publication already dispatched can complete, and completed files/recording bytes are not rolled back. Directory selection, retention and permissions remain their existing contracts. No hard revocation latency is promised.

Android DNS/bulk/reconnect reliability remains unresolved. Closed native tracing/baseline sequences are not resumed. Multies source, policy and release artifacts are unchanged.
