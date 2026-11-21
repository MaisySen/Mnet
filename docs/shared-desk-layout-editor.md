# Shared Desk offline arrangement editor

`mnet desk layout-edit FILE --out NEW_FILE` opens a native Linux X11 editor for
one validated arrangement. The read-only [inspector](shared-desk-layout-viewer.md)
shows a file; this editor changes an in-memory copy and can save it only as one
new explicitly named file. It is an offline editor: no pairing, run, daemon
request, trust update, input capture/injection or permission prompt. Labels stay
local file data, never verified people or devices.

## What the window does

- Shows the same aspect-preserving diagram as the inspector. The selected
  display is blue; a yellow strip marks the edge a new display would attach to.
- Shows the selected display's exact position, layout size and native pixel
  size from the original geometry, never rounded diagram pixels.
- Moves the selected display one layout unit per arrow key (Shift moves ten),
  clamped at the layout bounds. Moves along a shared edge keep the arrangement
  linked; moves that break every link are shown as invalid, not silently nudged.
- Changes layout size and native pixel dimensions with bounded +/- controls,
  clamped at the exact layout limits.
- **Add** attaches a same-sized copy of the selected display to the chosen
  edge, sharing that whole edge, with fresh bounded `display-N`/`device-N`
  identifiers. The copy may overlap a third display; that is reported as an
  invalid arrangement for the person to fix. Adding beyond 32 displays refuses.
- **Remove** deletes the selected display, refusing to go below two. Removing a
  link in a chain leaves a disconnected arrangement, shown with its reason.
- **Undo** restores earlier states, bounded to 64 entries. Undoing every change
  returns to the exact opened arrangement and a clean state.
- The opened file is an immutable in-memory snapshot. Replacing the file while
  the editor is open changes nothing.

## Validity and saving

Every state runs the exact `desk.Compile` gate. An invalid arrangement shows
its plain reason (overlap, disconnected, ambiguous edge, bounds) and Save stays
disabled with an explanation. Save requires unsaved changes and a valid
arrangement, then writes one strictly decodable document.

The destination is create-only: opened anchored to a checked private parent
directory (owned by this user, not writable by others), `O_EXCL`/`O_NOFOLLOW`,
mode 0600, file and directory synced before success. The source file already
exists, so it can never be the destination. An existing destination refuses
instead of replacing, with its bytes untouched. Save outcomes stay distinct: a
destination that was never created is reported as safely absent (nothing was
written), while a destination that was created but could not be confirmed is
reported as created-but-unconfirmed, never silently deleted, retried over or
claimed durable. After a successful save the window closes. Raw OS errors and the
destination path are not rendered into the window.

Closing with unsaved changes asks explicitly: **Discard changes** or
**Keep editing**. Escape or an outside press keeps editing. A clean close,
discard, SIGTERM, root geometry change or unsupported resize never writes.
Windows/macOS binaries reject the editor before reading any file; compilation
is not runtime UI support.

## Starting from no file

`mnet desk layout-new --out NEW_FILE` opens the same editor without an input
file. It seeds a two-display starter arrangement from the actual local root
pixel size: the local display carries the real pixel dimensions, and a second
placeholder display shares its full right edge. Both IDs and device labels
(`this-display`/`this-computer`, `next-display`/`next-computer`) are local
placeholders to edit, never verified devices. The starter is unsaved new
content, so the window opens dirty and valid with Save immediately available;
saving uses the same create-only path and closing without saving asks before
discarding. Nothing is paired, enabled or shared.

## Authority remains separate

This command makes no local daemon request, enrollment, network connection,
trust update, host approval or Shared Desk run. A saved file is only an
arrangement draft: the independent one-use pair, exact accepted layout binding
and both local reviews in `mnetd -desk-run FILE` still apply unchanged. The
[remembered-device design](shared-desk-remembered-device-design.md) boundary is
not implemented by this editor; nothing here is a saved pair or a trust store.

## Actual source evidence

Model race tests cover invalid-start refusal, move/resize/pixel clamping with
no-op detection, shared-edge placement on all four edges, identifier reuse
after removal, the 32-display cap and two-display floor, overlap/disconnect
invalidity blocking save, undo restore and the 64-entry bound, save-bytes
strict round trip, hit-target rules and original-snapshot isolation.

`scripts/desk-edit-validation.py` builds the race binary and runs the actual
window on both 1024x768 and 2048x1536 disposable X11 displays. It checks FIFO/
directory/device refusal before any window, usage refusals, exact one-unit and
Shift moves, cross-target release cancellation, mouse undo, edge selection,
attach, native pixel edit, create-only synced save with exact saved bytes and
0600 mode, source bytes unchanged, existing-destination refusal, the disabled
Save notice on an overlapped arrangement, the starter journey (no input file,
dirty-valid open seeded from the actual root size, create-only save with exact
placeholder bytes, explicit discard writing nothing), the explicit discard
confirmation
(mouse and keyboard, cancel and confirm), keyboard focus pixels and pending-key
Tab cancellation, immutable snapshot under file replacement, SIGTERM, root
resize and child resize teardown, and zero daemon calls on a trapped socket.
Current initial, attached, invalid-overlap, save-refused and confirmation
pixels were inspected. These are same-host synthetic geometry checks, not
paired physical displays, a consumer setup flow or persistent trusted pairing.
