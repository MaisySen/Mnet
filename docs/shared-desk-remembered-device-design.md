# Shared Desk remembered-device design boundary

This is a proposed product path, not a trust store or enabled pairing feature. Current one-run preparation and launch remain unchanged. Multies is read-only and the production identity/workspace verifier is absent.

## What remembering a device means

A remembered device can retain a locally chosen label and an exact previously compared public-key fingerprint. It is an identity hint for later comparison, not a grant. It must never contain a saved Allow decision, active run, recipient input approval, workspace membership assertion or reusable input capability. A remembered endpoint is a connection hint, not proof that the address still belongs to that device.

A key mismatch is a different device until deliberately replaced through the full comparison path. No reconnect, rename, same network address, familiar label or restored backup may silently replace the exact key. Forgetting removes the hint and cancels pending/active work for that exact pair; it must not remove an unrelated node identity or peer's saved records.

## Proposed actual journey

1. The installed app obtains fresh verified Multies user/workspace/device/capability evidence. Until the bridge exists, the consumer Add device and Enable actions remain unavailable. Local engineering inspection/prepare stays visibly separate.
2. Choose an eligible device. Obtain both current exact device-key fingerprints over an authenticated workspace/device route. Show a comparison on both actual local screens. A label, numeric address, same-network membership or remotely supplied claim cannot authenticate itself.
3. Each local person explicitly confirms that the exact keys refer to the intended devices. Only that local confirmation may save a remembered-key hint. It does not accept input or skip the next step. Record the exact key and a private label, not a broad account cookie or secret received from the peer.
4. Build and inspect the current arrangement from actual native display and keyboard-map evidence. Revalidate after display/map change. The current engineering launcher supports one source and one recipient with matching keyboard maps; consumer UI must not imply multi-monitor or mixed-layout runtime support from the more general offline geometry engine.
5. Explicitly enable one finite run. Bind fresh run ID, both keys, exact layout hash, keyboard hash and receiving display. Recheck current workspace and tool-specific capability. Both existing default-decline local reviews remain required. No capture/listener/input owner starts because the hint was loaded.
6. Keep exact current mode, visible local Stop and bounded run lifetime. Disconnect, hidden/closed required status, authority loss, native evidence change, local Stop or expiry retires that run and joins resource cleanup. A lost cleanup acknowledgement remains uncertain, never a green stopped claim. Reconnecting starts at fresh authority and run review, not saved approval.

## Persistence and ownership before implementation

The eventual record needs a versioned strict bounded schema, canonical keys, a bounded label, stable local record identity, and a clear platform-specific owner-only storage policy. Store operations need no-follow descriptor-bound reads, bounded content, atomic synced writes, explicit changed-record conflict handling and no automatic overwrite/import merging. The existing engineering run JSON is not a trust-store format. Private keys stay in their owning device's secure local storage and never travel with a remembered record.

UI confirmation, cancellation and late results must be bound to the exact record generation and key. A declined comparison creates no record. A canceled attempt cannot save a record or start a run after its window closes. Read corruption and unavailable storage deny saving/activation, not silently reset trust. Deleting local hints must not alter server membership or a peer's configuration.

The record's label may be displayed as local text only with length/control-character handling. It cannot become markup, a shell argument, identity proof or an authorization prompt supplied by the peer. Exact fingerprint and scope must stay visible at the decision point; a friendly label cannot replace them.

## Verification required before claiming saved pairing

A useful acceptance is the real installed app adding a pair on two separately controlled hosts, declining and accepting local comparison, saving only the intended public hint, restarting and loading it without capture/input, then independently approving a fresh run and observing exact native state/Stop. Wrong keys, changed addresses, duplicate labels, corrupt/symlink/oversized records, stale local answers, canceled saves, record replacement, forgotten active pairs and lost workspace authority must all deny or retire actual resources as specified.

Unit tests for a record codec alone would not close that journey. The current disposable-X11 one-run evidence does not close it either. This design avoids implementing another disconnected primitive or implying that a local pin store solves the missing production verifier. The next implementation decision belongs with the approved Multies bridge and installed local confirmation path, not an automatic extension of engineering inspection.
