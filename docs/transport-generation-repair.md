# Transport close/reopen ownership repair

18 November 2026. This is a reproduced standalone reliability defect, not an explanation of the unresolved Android DNS failures.

## Reproduction on the previous implementation

Fresh local tests demonstrated that:

- A duplicate Open silently replaced the active UDP socket without closing it.
- Relay packets buffered before Close survived into the next Open.
- Old UDP and relay receive callbacks consumed new-generation packets after Close/Open.
- Packets injected while closed were accepted into the queue and delivered after reopening.

The WireGuard conn.Bind contract requires receive functions returned by Open to return net.ErrClosed after Close. The tests use actual loopback UDP and controlled queue transitions, not Android traces, timing sleeps or a guessed kernel drop.

## Repair

Open now serializes socket ownership and rejects an already-open bind with conn.ErrBindAlreadyOpen. Each successful Open creates its own relay queue and cancellation channel. Both receive callbacks capture that generation's socket/queue/cancellation instead of consulting mutable current-generation fields. Close cancels that generation, closes its socket and clears the published port. Relay injection admission and queue selection share the lifecycle lock, so injections while closed do not populate a later generation.

No queue capacity, socket deadline, protocol, permission or routing policy was changed. The relay remains bounded and may drop when full, as before.

## Verification

- The original four reproduction tests failed before the repair, including both UDP and relay stale-callback subcases.
- All bind tests passed fresh under the race detector with 20 repetitions.
- Expanded tests cover 16 simultaneous Open attempts, failed-bind retry and 100 immediate close/reopen receiver-cancellation iterations per run.
- Bind, daemon, mobile and DNS packages passed ten fresh race-detector repetitions, including shared transport integration coverage.
- The full Go suite passed fresh with the race detector.

Local test evidence does not replace current CI or native device evidence.

## Deliberate limits

Local DNS is intercepted before WireGuard and does not use this encrypted transport. Normal upstream WireGuard rebinding also waits for its old receive goroutines. Therefore the stale-callback reproduction is not evidence that this defect caused the original Android initial/reconnect DNS failures or the intermittent reverse bulk timeout. Those readiness gates remain unresolved.

The previous Android trace sequence remains closed. Failed trace setup acknowledgements, missing independent-UID socket records, loose skb-pointer reuse windows and broad same-thread return arrays must not be treated as causal attribution. No new Android diagnostic run, release, Multies mutation, paid resource or personal-device change is part of this repair.
