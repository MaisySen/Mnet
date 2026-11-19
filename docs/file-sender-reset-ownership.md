# Sender ownership after a receiver cancels a file

19 November 2026, Australia/Sydney. Mnet source correction; no new consumer entry point or native consent claim.

## Defect

`SendFile` previously waited for receiver replies only after it had copied the complete offered source into the outbound mux and queued FIN. A receiver could accept and then reset a file stream while the shared viewing session remained healthy. The sender did not observe that reset during copying or an outbound-capacity wait. A new real-mux regression failed against that source because canceled file DATA remained queued even after `SendFile` returned its final error.

## Correction

After acceptance, one owned receive worker waits for the receiver's terminal reply while the main sender copies the bounded file snapshot. A receiver reset, close or early response cancels subsequent copy/enqueue work and wakes a sender waiting for shared outbound capacity. The worker is joined on every return; releasing the file stream wakes its blocking receive. The final acknowledgement wait remains30seconds after FIN, not30seconds from the start of a potentially slow large-file copy. The exact final digest must still match.

Failed transfers retire only their own queued OPEN/DATA/FIN output across priorities, return exact queued-byte charges, release the stream and attempt a best-effort reset. The queue order and charges for chat, screen or another file remain intact. The shared mux is not closed to cancel a single file. An already-dispatched filesystem read or socket write must return on its own; queued retirement is not secure erasure or recall of bytes handed to the operating system. A receiver can still lie about receipt; an acknowledgement is not proof that an endpoint is trustworthy.

No wire capability, public API, headless default, mobile destination, per-offer consent rule or Multies policy changes. The [local per-offer approval boundary](file-offer-approval.md) remains a separate opt-in receiving gate, not a completed native chooser.

## Evidence

- A64MiB real-mux sender/receiver test first failed because canceled file DATA remained queued. It now requires prompt failure after reset, no canceled DATA in retained queues, live chat and a later successful file on that same session.
- A deterministic full-capacity test waits until the sender is actually in the condition-variable wait, then cancels and verifies it wakes without enqueueing. Queue-retirement tests preserve other streams, order and exact accounting, including idempotence.
- These tests plus source snapshot/mutation, owned-copy and per-offer approval tests pass20 fresh race repetitions. Full-repository race tests and vet pass. The first full run hit a DNS test's TCP-port bind collision; inspection confirmed UDP bind failure returns before TCP startup, the isolated case passed20 repetitions and a fresh full-repository run passed. No DNS implementation was changed.
- Credential/style and diff checks pass. Current cross-platform automatic CI is pending. No new native/emulator/device workflow or personal-device acceptance was performed.

## Remaining gates

The local receiver approval checkpoint611846f passed all five normal CI jobs in [run44493356722](https://github.com/MaisySen/Mnet/actions/runs/44493356722). Its native chooser, per-file user experience, destination persistence, lifecycle acceptance and headless migration decisions remain open. Multies identity/revocation integration, Android reliability/device acceptance and signed iOS distribution are not completed by this transport correction.
