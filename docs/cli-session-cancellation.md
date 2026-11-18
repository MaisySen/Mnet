# Terminal viewer EOF owns pending connection work

The interactive CLI viewer already ended an active session or reconnect-backoff on stdin EOF, but a pending TCP dial/host hello could wait its whole budget before observing it. The stdin scanner could also remain blocked trying to enqueue another chat line after the run ended. The new run context is canceled by scanner EOF/exit and by sessionConnect return.

TCP dial uses that context. A connection-close callback owns the pending authorized hello and active mux until the mux closes; it is then removed. Cancel-before-start refuses work, cancel during hello closes that socket, and no canceled attempt may publish a late mux. The bounded queue send now observes cancellation. Existing prompts, non-empty chat behavior, peer pin/current-evidence checks, time budgets and one-shot send semantics are retained. The separate one-shot sender uses a background lifetime because it has no interactive stdin ownership.

This cannot interrupt an arbitrary Reader blocked inside Read without closing shared stdin; the application does not take ownership of the user's stdin descriptor. The local authorization lookup has its existing two-second request budget and is not converted into a context-aware lookup in this unit. It may finish after EOF, but a canceled dial/hello cannot remain active afterward. A chat/file operation already dispatched can finish; transport closure is not rollback. EOF still ends the run instead of promising to deliver all buffered chat lines.

## Verification

Four real-loopback/queue tests pass30fresh race repetitions. EOF during a minute-budget silent host hello returns and closes the host socket; active run cancellation closes its mux but not an unrelated socket; already-canceled work cannot look up/dial/hello; a scanner blocked on a full queue exits and ordinary prompts/lines remain unchanged. An overlay omitting socket cancellation fails the blocked-hello regression. Fresh full repository uncached race tests and vet pass locally. Desktop CI now repeats these tests on macOS and Windows; exact changed-revision general results must be collected separately.

No public-network, desktop capture/input, physical-device or native Android reliability claim follows. No peer recipient, admission policy, route/binding, budget, file consent/retention, Multies integration, signed artifact, live service or release changes are included.
