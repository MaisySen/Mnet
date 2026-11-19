# Android document-output adapter preparation

19 November 2026, Australia/Sydney. Disabled source preparation and simulated framework evidence, not a working consumer file destination.

## Implemented boundary

`DocumentExport` is package-private. The newer unregistered selection Activity can construct it, but no registered Activity, service, mobile or Go entry point calls it. No enabled picker, manifest component, persisted URI permission or native descriptor bridge is added. The existing receiver path/defaults and disabled consumer/mobile entry points are unchanged. It is preparation for, not an implementation of, the [receiver export-capability contract](file-export-capability.md).

The normal constructor accepts a local `ContentResolver`, a content URI and the expected byte count. A future caller must supply a fresh document selected for the exact local offer. The helper cannot prove a URI came from a picker, denotes a new document, or belongs to an authorized offer. Passing a remote-supplied URI or an existing document would violate the intended boundary. There is deliberately no public/native caller until that binding and lifecycle are implemented and reviewed.

- Open is one-use, with a `CancellationSignal`. Null, refused and late-after-cancel opens fail. A descriptor returned after cancellation is closed.
- Java owns the `ParcelFileDescriptor` throughout. The normal output is Android's `AutoCloseOutputStream`. No borrowed integer descriptor, `detachFd`, URI-to-path conversion, seeking or sync is used.
- A single owner worker calls open/write/finish in order. Cancellation may be concurrent. The helper rejects writes beyond the expected count and finish before the exact count. Digest verification remains the separate Go staging contract, not something this unused helper performs.
- Completion returns only after the local stream closes successfully and no cancellation was observed at the final ownership check. Known cancellation/failure closes the acquired descriptor and refuses later completion. Provider errors are not surfaced with URI or private exception details.
- Cancellation signals the open and attempts `closeWithError`, followed by idempotent close even if error signaling fails. These are best-effort resource/error actions, not deletion, rollback, recall of written bytes or a hard deadline for a blocked provider call.

Mode `w` gives providers streaming flexibility but does not have uniform truncation semantics. Only a future fresh-document selection contract can avoid treating this as safe overwrite. `ACTION_CREATE_DOCUMENT` is the intended selection primitive; it is not wired here. Nothing deletes the selected URI on error. A failed operation can leave an empty or partial document, and cleanup/recovery UX remains an enablement gate.

## Ownership sources

The implementation decision is grounded in Android platform source pinned to the Android14 release tag, rather than a moving master branch:

- [ContentResolver, android-14.0.0_r1](https://android.googlesource.com/platform/frameworks/base/+/android-14.0.0_r1/core/java/android/content/ContentResolver.java): nullable caller-owned descriptor, cancellation, provider-specific mode behavior and streaming descriptors.
- [ParcelFileDescriptor, android-14.0.0_r1](https://android.googlesource.com/platform/frameworks/base/+/android-14.0.0_r1/core/java/android/os/ParcelFileDescriptor.java): AutoCloseOutputStream owns close; closeWithError only informs an observing peer; detaching changes ownership and can lose reliable close/error signaling.
- [Android document selection guide](https://developer.android.com/training/data-storage/shared/documents-files): user-selected document operations and create-document no-overwrite behavior.

This source tag is an API/ownership reference, not a claim that the tested API24/25/26/33 simulations run Android14. Real OEM/provider implementations can differ.

## Reproduction and evidence

With JDK17 and Maven available:

```sh
JAVA_HOME=/path/to/jdk17 MVN=/path/to/mvn sh scripts/test-android-framework.sh
python3 scripts/test-android-api-compat.py
```

The selected framework suite passes252 cases, including52 new DocumentExport cases,13 tests on each of APIs24/25/26/33. They use the actual helper and ContentResolver against an explicitly registered in-process fake ContentProvider. Cases cover exact bytes, one-use open, short/oversized transfers, null/refused/late open, cancellation before open and during open/write/close, invalid selections, read-only write failure, descriptor release and later-call refusal. Narrow output-factory fakes inject construction, write, close and noncooperative-write/close faults. They do not establish that an actual provider fails or cancels in the same way.

Robolectric4.18's [ParcelFileDescriptor shadow](https://raw.githubusercontent.com/robolectric/robolectric/robolectric-4.18/shadows/framework/src/main/java/org/robolectric/shadows/ShadowParcelFileDescriptor.java) implements createPipe with a temporary file. The explicitly named simulated-pipe case therefore proves no helper seek/sync requirement, not kernel pipe blocking, reliable peer error delivery or real provider streaming. Descriptor cleanup tests needed JDK17 java.io reflection access, configured in this fixture's Maven process only. Initial failures came from missing fake-provider attachment and that test-runtime reflection restriction; neither is reported as a platform defect.

No visible layout changes or new screenshots are claimed. Normal CI for this new checkpoint remains pending until reported separately.

## Newer routing preparation

The [disabled selection Activity](android-document-selection-ownership.md) now prepares one-instance result routing and temporary-grant lifetime ownership without a manifest registration or production caller. The Go bridge and enabled reception remain absent. Its framework cases do not prove installed permissions or provider behavior.

## Still open

Real chooser generation/offer binding, grants/revocation, Activity/process death, provider crash/restart, actual pipes and reliable errors, partial-document recovery, installed permission enforcement, bounded native bridging, provider block/cancel behavior, timeout UX and any durability promise need their own acceptance. Closing a local output does not prove provider publication, remote persistence or cloud synchronization. The internal Go export seam is still unconnected to this helper. Multies, devices, signing, releases and production services are unchanged.
