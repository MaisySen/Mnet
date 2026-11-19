# Half-closed receive ownership and late file cancellation

19 November 2026, Australia/Sydney. Mnet transport source correction, not a native/provider acceptance result.

## Confirmed gap

The prior mux deleted a stream's dispatch entry when peer FIN arrived. FIN correctly ended file DATA, but receiving verification/export work could still be active. A later peer reset then found no entry and was dropped. A new real-mux test first failed against that source: an export blocked in Finish did not retire after reset, although the shared session stayed alive.

## Correction

FIN now closes the receive DATA side once and retains the stream entry until its local owner releases final processing. EOF remains available to the receiver. Duplicate FIN is harmless; post-FIN DATA is ignored rather than sent into a closed channel. Half-closed entries still count toward the existing256-entry active-stream bound, so unfinished local ownership cannot create an unbounded table. Owner release or whole-session Close retires entries and budget as before.

Peer reset has its own terminal signal, including after FIN. It does not wait to enqueue behind a full DATA queue. The owner sees known reset before queued DATA/EOF. FileReceiver checks that signal at its authorization/storage boundaries, and the disabled staged-export context watches it alongside whole-session End. A late reset during cooperative Open/Write/Finish retires export and prevents a successful completion/notification without closing viewing/chat. Already-entered noncooperative calls must still return or honor cancellation; this does not roll back bytes already delivered to a provider or published to a filesystem.

No wire format, native descriptor ownership, chooser, production provider, consumer/mobile reception, headless default, Multies authority, device route or release behavior is changed. The internal export seam remains disabled outside local test adapters. A final acknowledgement lost after publication remains an uncertain sender outcome, not evidence that bytes were undone.

## Evidence

- Actual mux/receiver export test first failed old source with a blocked Finish surviving post-FIN reset. Corrected test requires prompt retirement, adapter release, staging cleanup, no success notification and live chat on the same connection.
- A full256 half-closed streams remain owner-bound and counted toward capacity; local new admission refuses at cap. Releasing owners clears the table/budget. Duplicate FIN and post-FIN bytes cannot panic or add retained DATA charges. Later reset changes a prior EOF into reset for the active owner.
- Full inbound DATA queue test proves reset reaches the owner without waiting for DATA consumption and does not stall another stream's dispatch; releasing returns queued-byte charges.
- Earlier tests were reconciled to the intended ownership change: the FIN budget test waits for the explicit FIN signal and asserts ownership remains until release, and the full-queue dispatch test retains its DATA blocking case while the new reset test verifies nonblocking terminal dispatch. No budget or cap check is dropped.
- Twenty focused race repetitions pass, including half-close/reset/export/inbound budget/release/repeated-file/receive ownership families. Fresh full-repository race tests and vet pass; credential/style and diff checks pass. Current normal cross-platform CI is pending.

The preceding disabled export baseline1b78155 passed all five normal CI jobs in [run44507446267](https://github.com/MaisySen/Mnet/actions/runs/44507446267). This current correction still needs its own normal CI. Native/provider/picker/process/timeout-UX/durability and consumer enablement gates remain open.
