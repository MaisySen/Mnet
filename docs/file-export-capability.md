# Disabled local export-capability seam

19 November 2026, Australia/Sydney. Internal Mnet source preparation, not a native provider or enabled file-reception feature.

## Scope

The live receiver can now carry an internal, one-use opaque export selection through its exact local approval request. No public/native API or production caller creates one. A test adapter drives the real mux, approval and receiver path. Existing public directory approval, direct filesystem backend, headless configured-directory default and absent mobile/viewer destinations are unchanged. No URI, raw fd, native chooser, provider grant or consumer screen is added.

Each selection is consumed by one exact current request, including session, stream and immutable offer metadata. Invalid answers cannot consume the real prompt; reuse cannot approve another identical offer. Path decisions retain their existing implementation. Export decisions specify an absolute local staging root, never a document URI or remote-selected destination.

## Receive and export ownership

After current local approval, the receiver reserves staging capacity and creates one private0700 subdirectory with a0600 temporary file. It checks exact offered size and optional offered digest using the existing receive path. Only after verification and staging finalization does it call the selected adapter's Open, Write and Finish. Invalid bytes never open the export destination. No positive final acknowledgement or completion notification occurs before observed successful Finish.

The capability/output interfaces are unexported local contracts. They carry a context through Open/Write/Finish rather than transfer ownership of a borrowed descriptor. A returned output is always aborted/released, including a partial resource returned alongside an error and after successful Finish. Finish owns the adapter's observable close result; subsequent Abort must release remaining resources without deleting successful output. Nil output, short write, open/write/finish error, lost session ownership or End cannot report success. Adapter errors are not forwarded to the peer.

One receiver owns staging and one export attempt. A joined watcher cancels its context on session End. Ownership is checked around new reads, writes and Finish. Cooperative export has a20second context deadline starting after full verification, before the existing sender's30second final-ack wait. A call already entered must honor cancellation or eventually return; a context cannot forcibly interrupt arbitrary native/provider code. A late returned output is still released and cannot revive success. The context bound is development-source behavior, not a completed picker/export timeout UX.

Per approval gate, at most eight staged exports and1GiB aggregate declared bytes can be reserved. Zero-byte offers consume an active slot. Oversize/invalid reservations fail, and cleanup returns reservations exactly once. This budget is independent from pending-prompt capacity and preserves the path backend's current policy. The root may remain as an empty local directory; per-transfer staging is removed best-effort after success/failure. Cleanup is not secure erasure or a device-wide storage quota.

## Evidence

Initial new-capability tests did not compile against the prior API. That is missing-capability coverage, not a claim of an older intended-default regression.

Fresh full-repository race tests and vet pass. Twenty fresh repetitions of file export/approval/storage/path/sender/owned-copy cases pass, including:

- Exact verified bytes and metadata through the real mux/receiver.
- No staging/adapter effect before approval, invalid-digest refusal before Open, changed request/relative staging refusal, one-use selection and identical sibling isolation.
- Nil/failed/partial-resource Open, failed/short Write, failed Finish and expected resource-release order, without success acknowledgement/notification on failure.
- Actual End during blocked Open/Write/Finish. Cooperative calls observe cancellation; a deliberately noncooperative Open returns late and its resource is still released. A blocked successful Finish prevents sender completion until released.
- Private staging modes, eight-active refusal, declared-byte overflow refusal, exact reservation return and a later successful transfer after capacity recovery.
- Existing filesystem no-overwrite/private-mode/cleanup and sender-reset behavior remain covered.

Baseline1b78155 passed all five normal CI jobs in [run44507446267](https://github.com/MaisySen/Mnet/actions/runs/44507446267). No additional native/emulator/device workflow, new pixels or installed-provider acceptance is claimed.

## Unresolved native and product gates

No Android adapter exists. Android documentation/source verifies that document URIs are not directory paths, descriptor output can be a pipe/socket, getFd retains Java ownership, detachFd can lose reliable close/error signaling, and closeWithError does not imply rollback. These constraints inform the internal contract but do not prove an actual provider implementation.

Sources read directly:
- https://android.googlesource.com/platform/frameworks/base/+/master/core/java/android/content/ContentResolver.java
- https://android.googlesource.com/platform/frameworks/base/+/HEAD/core/java/android/os/ParcelFileDescriptor.java
- https://developer.android.com/training/data-storage/shared/documents-files
- https://developer.android.com/guide/topics/providers/document-provider

The final native implementation must pin its SDK/source, select and verify descriptor/stream ownership, bind picker lifecycle/results to the exact pending request, and test inaccessible/moved/crashed providers. A provider's observed local Finish is not atomic publication, cloud durability, rollback or proof that a remote endpoint is trustworthy. Those claims cannot be inherited from the path backend's hard link.

Picker lifetime versus the existing20second local approval/30second sender acceptance waits is not solved. The original baseline removed receive dispatch on FIN, dropping a later reset during export. A subsequent [half-close/reset ownership correction](stream-halfclose-reset-ownership.md) retains bounded owner state until release and carries reset to cooperative finalization without ending shared viewing. Its source checks are separate from baseline CI and still do not establish provider/native behavior. Cancellation cannot recall bytes already handed to a provider.

Consumer/mobile reception, native chooser/provider/process acceptance, persisted/background grants, headless default migration, Multies authority, device routes, signing and release remain unchanged and open.
