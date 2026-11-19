# Local per-offer file approval boundary

19 November 2026, Australia/Sydney. Mnet-only source checkpoint, not an end-user chooser or Multies integration.

## Implemented scope

The active `FileReceiver` path now has an explicit `RequireApproval` mode and a concrete local `FileApprovalGate`. `HostOptions.FileApprovalRequired` and `HostOptions.FileApproval` configure it before stream dispatch. This is wired into real transfer/storage code and exercised over real mux frames, rather than a separate unused consent state machine.

In required mode, an absent, zero-value, closed, exhausted or wrong-session gate denies reception. A fixed directory, viewing approval, input grant, peer payload or chat message cannot substitute for a local file answer. After metadata validation, the receiver emits a local-only request containing immutable offered name/size/digest, a fresh random request ID, a gate/session ID, exact stream ID, deadline and retirement channel. The gate binds once to one mux. A later session needs a fresh gate. The exact original request must be returned to `Decide`; altered metadata, changed session/stream/request, replay and a prior request's answer cannot consume the current prompt.

Approval supplies an absolute local directory through the local method. The peer cannot pick that directory. Nothing creates a directory, temporary file or positive acceptance before the answer. Decline, timeout, session End, early data/FIN/reset, local release and observed authorization loss retire pending work. `Done` lets a future UI dismiss stale prompts. A selected answer is rechecked against terminal state, deadline, early peer traffic and current session authorization before entering storage. Existing repeated authorization, size/digest checking, private temporary storage and atomic no-overwrite publication remain in place.

The gate uses channels and bounded local state, not an arbitrary blocking UI callback. At most eight requests may be pending, and at most eight notifications may be queued for the future UI. Decision waits are capped at20seconds, before the existing sender's30-second acceptance wait. Nonpositive timeout selects20seconds; callers may choose a shorter wait. Authorization is polled every100milliseconds while waiting and checked again at approval/storage boundaries; this is not an instant-revocation or scheduling-latency promise. Existing authorization callbacks must still return. A retired queued notification can remain until drained; consumers must check `Done` before presenting it. No drain/capacity means fail closed, not silent evictions or unlimited UI work.

Approval-refusal output does not wait for an unresponsive peer to drain the outbound queue. It attempts a fixed refusal and then best-effort stream reset if capacity is unavailable. It never grants on an output failure. Request formatting redacts correlation handles/offer metadata, and JSON encoding exposes none of the local request fields. Do not serialize a request into protocol authority or trust a remote copy as a local decision.

## Preserved boundaries

The zero-value `RequireApproval` is false. It preserves the existing explicitly configured-directory engineering policy, including the headless daemon's received-files directory. That legacy mode is not per-file consumer consent. A future migration must select its behavior explicitly rather than silently breaking automation or labeling a directory as approval.

Mobile host and default viewer reception remain disabled because their destinations remain unset. No screen, destination picker, OS storage grant, new entry point, protocol permission, default policy or CLI option is added. A local test adapter is not a native consent UI. No Multies identity/workspace authorization is added. The bridge, signed delivery, device routes and release decisions remain separate.

A prompt's local correlation binding authenticates no human by itself. Future UI adapters must bind their caller/context to the actual authenticated session, show untrusted offer metadata safely, select/persist a supported destination and dismiss retired prompts. A completed local answer does not grant future files, recording or control. Already-published files are not deleted by later revocation, and this change does not promise secure erasure or rollback of dispatched filesystem operations.

## Evidence

- The initial focused tests did not compile against the old receiver because the required-approval API did not exist. This is new capability coverage, not a claim that an older intended default regressed.
- Twenty fresh race repetitions pass for missing/zero provider, exact-byte approval, altered request/session/stream/name/size/digest, relative destination refusal, replay/replacement, identical concurrent offers requiring separate answers, bounded pending capacity, gate reuse across sessions, decline/timeout/End/reset/early data/authorization loss/gate close, and actual host Stop while awaiting approval.
- Actual `ServeHostWithOptions` tests prove the required mode uses only the locally selected directory, while the unchanged legacy default uses its configured directory without prompting.
- Invalid name/size/digest offers never reach the local decision queue. Approved-mode destination collision, digest mismatch and overflow keep existing bytes and remove temporary files. Local request display/JSON redaction and zero-gate refusal are covered.
- Fresh full-repository `go test -race -count=1 ./...` and `go vet ./...` pass. The first broader pass caught an extra authorization poll disturbing legacy boundary-test call order; the new poll was moved into opt-in mode and the full suite reran successfully. Credential/style and diff checks pass.
- Normal automatic cross-platform CI at611846f passed all five jobs in [run44493356722](https://github.com/MaisySen/Mnet/actions/runs/44493356722). No new native/emulator/device sequence was started. There are no new UI pixels to accept because this change exposes no end-user UI.

## Still needed

A real attended receiver needs a local chooser and file-specific decision surface, reliable prompt cancellation under its native lifecycle, supported destination access/persistence, user-visible outcomes and installed-platform acceptance. Headless default migration requires an explicit policy choice. Production Multies authority and membership-loss propagation remain outside this local gate. None of these gates is cleared by the Go tests.

A later [sender reset-ownership correction](file-sender-reset-ownership.md) stops failed-transfer copy/queue work without ending unrelated session traffic. It does not add native file consent.
