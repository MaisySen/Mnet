# Export-selection release before output open

19 November 2026, Australia/Sydney. Actual receiver/mux ownership correction. No mobile route or native reception is enabled.

## Defect

An approved document selection can already own native resources before its output opens, including a selected document handle and the prepared Android selection Activity's lifetime slot. The internal export capability previously exposed only Open. The receiver cleaned an output returned from Open, but had no capability-level release when Open never ran.

That misses failures before export: authorization loss after a queued answer, staging-budget refusal, staging-directory setup failure, reset while receiving the private stage or digest mismatch. A queued destination can also lose the approval waiter's select race to peer data/reset/End before it is consumed. The old source never notified the selection owner to release in those cases. The Android route is still disabled, so this is a prerequisite source defect, not evidence that a real user/provider resource leaked.

## Ownership fix

The internal capability now requires Release in addition to Open. Selection release is one-time and separate from output Abort. It must release selection-owned resources even without Open, and be harmless after successful output completion. It is not a request to delete a completed document or roll back bytes.

- Rejected/malformed/stale submissions remain caller-owned. They cannot consume or release another local capability. The caller releases its rejected selection.
- A successful queued gate decision transfers the selection to the approval waiter. The waiter retires its gate entry before draining an abandoned queued answer, synchronizing against late decisions.
- If the waiter consumes a selection but cannot return an approved destination, it releases that selection.
- A successful approved-destination return transfers release ownership to FileReceiver. It defers one-time release before its next authorization/storage check, covering every later return, including failures before staging or Open.
- A released selection cannot be claimed or started again. Release calls the native capability outside the selection/gate locks.

Prompt Done and export lifetime remain distinct. A successful gate decision closes the prompt's Done, but does not release the selected native capability while staging/export still owns it. Stream/session terminal state and the receiver's eventual exit retire the capability. Arbitrary native cleanup can still block or fail; a one-time call is not proof of forced provider cancellation or real OS resource reclamation.

The ordinary path destination, headless defaults, public gate Decide semantics, consumer/mobile entry points and manifest are unchanged. A private native-shaped callback adapter is included below; no callable mobile decision object or runtime native bridge is exposed.

## Private synchronous native-output adapter

The unexported Go nativeFileExport adapts a native-owned Open/Write/Finish/Cancel shape to the real receiver capability/output contract. It has no mobile/public constructor, registered provider, JNI implementation or app caller. The existing Java output matches this shape through a compile-only wrapper experiment, not a connected runtime bridge.

The adapter binds one original export context, rejects calls before Open/reopen/after Finish/release, calls native cancellation once when that context ends, and joins its watcher during receiver cleanup. Native methods are synchronous full-write-or-error and cannot retain or mutate supplied bytes. A native exception/failure or a late return after cancellation never becomes successful completion. Open returning partial ownership still returns a cleanup handle. Native Cancel is terminal, must own late acquisitions itself and be harmless after successful Finish. Java's prepared DocumentExport has its own late-descriptor cleanup; Go does not take a borrowed fd.

A native call or Cancel that ignores cancellation can still block until it returns. Native Cancel must not synchronously wait for its own Go export worker to finish. These cooperation requirements and real JNI exception/byte-array/thread behavior remain acceptance gates, not hard timeout promises. The new adapter does not solve the separate private decision/Done synchronization or enable a mobile callback route.

Real receiver tests exercise native-shaped success, Open/write/finish failure, End during blocked calls and late successful returns after cancellation. They assert no success notification, one native cancellation/release and a joined watcher. These use Go callback fakes, not Java/provider execution.

## Regression evidence

Six new real mux/receiver cases first fail against the old source with zero capability-release calls: reset during staging, exhausted budget, invalid staging root, authorization loss after queued choice, digest mismatch and successful completion. After the fix each releases its selected capability exactly once. The pre-open failures do not invoke provider Open. The successful-choice case also checks that prompt Done closes without prematurely releasing the selection.

Other cases verify caller ownership after a rejected decision, refusal to reuse a released selection, and100 concurrent decision/DATA/reset interleavings per run with exactly one final owner release. The full export/approval families pass20 race repetitions, including2000 such interleavings. Existing real-mux output Open/write/finish failures, late returns, exact verified bytes, staging/private modes, half-close/reset and unchanged path tests remain in those families.

Run:

```sh
go test -race -count=20 -timeout=2m ./internal/session -run '^TestFileExport|^TestFileApproval'
```

Full repository race/vet and current normal CI status are reported separately. These Go tests use explicit local capability/output fakes. They do not execute Android grants, a picker, a provider, a native callback bridge or a device. The separate disabled Android selection/output framework evidence keeps its own limits.

## Related source contracts

- [Disabled export capability](file-export-capability.md)
- [Android selection lifetime](android-document-selection-ownership.md)
- [Android descriptor ownership](android-document-export-preparation.md)
- [Half-close and reset ownership](stream-halfclose-reset-ownership.md)


## Later private exact-decision preparation

The [private native decision adapter](file-native-decision-ownership.md) now retains the original exact gate/request and synchronizes successful Choose with its prompt Done watcher. Explicit owner close and receiver cleanup share one-time selected release. It adds no exported mobile/native route and does not execute JNI or Android callbacks.
