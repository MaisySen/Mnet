# Control status callback reentry

The viewer's attended-control callback reports current state to the application. It must be able to react by canceling its own pending request or ending its gate. Previously notification held a mutex while calling application code. Those reactions reentered notification and deadlocked. Two regressions failed the old source, including cancellation over an actual session mux.

Notification now has one bounded serial drain. State changes mark a pending update and the active drain reads the latest snapshot. It never holds the state or notification mutex while calling application code. A reentrant or concurrent update is coalesced for the same drain, not given another callback goroutine or an unbounded queue. End is idempotent, so an application reacting to terminal state cannot recursively generate terminal notifications.

A blocked-callback regression requests, cancels and calls End1,000times while application code waits. These operations finish without waiting for that callback, input remains disabled, and release delivers the latest unavailable state without replaying intermediate pending/declined states or overlapping callbacks. The ordinary attended grant, pause, resume, revoke, stale-answer, canceled-request, backpressure and view-only tests still pass.

Twenty focused race repetitions, the full repository race suite, vet and source checks pass locally. Existing native session CI now explicitly selects the callback regressions. Hosted verification for this source remains separate and pending. This is protocol/application callback ownership, not native UI, physical consent/recovery, Multies integration, production deployment or release acceptance. No grant rules, protocol fields, default capability or native entry point changes.

```sh
go test -race -count=20 -timeout=2m ./internal/session \
  -run '^TestViewerControlCallback|^TestAttendedControl|^TestControl'
go test -race -count=1 -timeout=5m ./...
go vet ./...
```
