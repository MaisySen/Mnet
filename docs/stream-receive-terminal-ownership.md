# Stream receive terminal ownership

Stream.Recv used one select for buffered DATA/FIN and mux closure. When both were ready, it could return buffered work or normal EOF after the mux had already ended. A locally released stream also left its Recv waiter blocked until a frame, timeout or whole-session closure. Release after a peer FIN could miss closing the stream's done channel because FIN had already removed its table entry.

Recv now checks mux and local-release signals before waiting and after selecting a frame/FIN or timeout. Known terminal state returns io.ErrClosedPipe instead of delivering retired work. A frame selected while closure occurs still returns its inbound accounting before being refused. Positive timeout timers are stopped on return; nonpositive timeouts still have no deadline. Live DATA, peer reset and normal FIN behavior remain unchanged.

Stream release now closes the done signal in the same idempotent accounting operation that retires the stream state. This also covers release through an existing handle after FIN removes the table entry. It wakes that stream's waiter without closing unrelated streams or the mux. The existing budget regression now expects released-stream reads to fail rather than deliver stale queued data.

## Evidence and limits

Four receive test families and the inbound-budget regressions pass 30 fresh local race repetitions. They cover queued DATA/reset/FIN with mux closure or local release, release after FIN, indefinite/hour-deadline waiter wakeup, normal live DATA/reset/EOF, and closure after DATA selection but before accounting completes. The latter uses an unbuffered handoff plus the accounting mutex, not a sleep, to force the late-delivery ordering. A source overlay restoring old Recv/release behavior fails terminal precedence, local-wakeup and post-selection checks. Full uncached repository race tests and vet pass locally.

Desktop CI includes 30 repetitions of the receive families on Windows/macOS; collect the exact source revision before claiming platform results. This is fixture/source validation, not native phone networking acceptance. Terminal checks are not rollback: closure after the final check can race a caller's subsequent work, and previously returned data or an already-dispatched callback is not recalled. Mux closure does not itself join every application consumer. No wire format, consent policy, destination policy, Multies integration, signing/account/device operation or release scope changes.
