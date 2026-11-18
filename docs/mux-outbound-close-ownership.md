# Outbound queue ownership when the mux ends

Mux.Close previously closed the transport and woke blocked producers but left its outbound queues, byte counter and implicit-open markers attached to the mux. A retained closed session could therefore retain queued screen/file payloads up to the existing queue limit. Its writer also popped available work before checking terminal state.

Close now drops the queued frame slices, resets queued-byte accounting and clears implicit-open markers under the outbound mutex before waking producers. The writer checks terminal state before selecting another queued frame. Dynamic Open also rechecks closure under that mutex before publishing its marker, so an enqueue/Close race cannot restore markers on the retired mux. Active-session backpressure and priority scheduling are unchanged.

## Evidence and limits

Two focused test families plus the existing blocked-producer regression pass 30 fresh local race repetitions. They fill the stopped-writer queue, end the mux, verify that queue backing arrays/counters/markers are gone, check producer wakeup and refused late sends, and confirm a terminal writer does not dispatch queued work. The second family races Open/Close 100 times per test and checks terminal outbound ownership. Restoring the old implementation through a source overlay reproduces retained queued buffers/markers. Full uncached repository race tests and vet pass locally. Desktop CI includes the two new families at 30 repetitions; exact-revision results must be collected.

This does not promise secure erasure, zero total process memory or cancellation of an OS write already handed to the socket. One in-flight frame, application-owned frame copies, inbound stream queues and references outside the mux have separate lifetimes. Connection Close still interrupts pending network I/O; already-returned or peer-received data cannot be recalled. No wire format, active queue size, user consent, destination/retention policy, Multies integration, native-phone acceptance, signing/account/device operation or release is included.
