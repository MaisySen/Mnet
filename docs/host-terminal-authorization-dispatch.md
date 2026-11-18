# Host authorization after terminal session state

The host's allowed check previously tested mux closure before invoking its authorization callback, but a callback returning true after session End could still pass. This let a waiting input/chat/capture worker dispatch one operation after known mux closure. The host Stop signal was also not part of this shared guard, leaving its short pre-mux-close interval unchecked.

The shared guard now checks known denial, Stop and mux closure before authorization and again after the callback. A late true result cannot revive that host. A false result still permanently marks it denied and closes the transport. The guard applies at existing host input/chat/capture/file/recording dispatch boundaries; it does not grant or widen authorization.

## Evidence and limits

Two test families pass 30 fresh local race repetitions. A blocked authorization callback is released only after mux End, Stop or a concurrent denial, with input, chat and screen-capture workers. No injector/chat/capture operation is dispatched and the worker returns. A separate family verifies known terminal state skips new authorization callbacks. The Stop/denial cases isolate those terminal signals without relying on timing inside Stop. The old-source overlay reproduces late input, chat and capture dispatch, plus workers waiting after the Stop signal. Full uncached repository race tests and vet pass locally.

Desktop CI includes these families at 30 repetitions on macOS/Windows; collect exact-revision logs before reporting platform success. This is framed/local worker evidence, not physical input/capture or integrated Multies acceptance. An authorization callback already underway is not forcibly interrupted, and a terminal race after the final guard cannot recall an operation already dispatched. Host Stop can still wait for an external callback/backend to return. No wire format, consent UI or policy, destination/retention rule, Multies mutation, native Android experiment, signing/account/device action or release changes.
