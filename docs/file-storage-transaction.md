# Receiver-owned storage transaction preparation

19 November 2026, Australia/Sydney. Mnet-only internal refactor and fault tests. No native destination adapter or file reception is enabled.

## Implemented

The live FileReceiver now uses an internal transaction for staged Write, Finalize, Publish and Abort. The default path implementation retains the same directory creation, existing-target refusal, private0600 temporary file, byte/digest checks, sync/close and atomic no-overwrite hard-link publication. Abort always runs and closes/removes only staging; it never deletes a completed destination. Session-authorization checks stay at the existing receiver boundaries before directory work, writes, finalization, publication and completion notification. The opt-in file approval gate still supplies the directory only after a current local answer.

This is not a new public/native storage API. External callers retain only the existing path backend. A private receiver construction seam supports deterministic fault tests. There is no production document-provider implementation, descriptor transfer, chooser, destination persistence, new mobile callback or consumer entry point. Existing headless defaults are unchanged.

The internal contract makes a future adapter's ownership visible: one receiver worker owns one transaction, short writes fail, Finalize must succeed before Publish, publication failure cannot send success, and Abort must safely release staging after both failure and success. A session ending during Finalize is rechecked before Publish. It does not promise cancellation of a filesystem/provider operation already dispatched, secure deletion or generic provider rollback.

## Why the native adapter is separate

The Android [Storage Access Framework guide](https://developer.android.com/training/data-storage/shared/documents-files) returns document URIs, not filesystem directories. ACTION_CREATE_DOCUMENT gives per-file Save As, cannot overwrite an existing file and may alter the displayed name for a collision. ACTION_OPEN_DOCUMENT_TREE instead grants access to a whole directory and descendants. [Document-provider guidance](https://developer.android.com/guide/topics/providers/document-provider) describes provider-specific backing storage and supported capabilities. Passing a selected URI into the current absolute-directory gate would be wrong, and a selected URI is not proof of atomic/no-overwrite export.

The bounded future proposal is one-file local approval, no persistent tree grant, exact service/session/offer/picker-generation binding and a verified private staging/export adapter. It must reconcile the20second local approval and30second sender acceptance deadlines with potentially slow native selection. Late result, process/Activity replacement, End, descriptor ownership and provider failure need explicit handling. These are engineering tasks, not a reason to repeat the deferred Multies question. Enabling consumer reception and installed/provider acceptance remain separate effects and gates.

## Evidence and limits

- Existing file receiver/sender/approval families pass20 fresh race repetitions after extraction. Fresh full-repository race tests and vet pass, plus credential/style/diff checks.
- Real-mux injected transaction cases cover normal exact bytes, write failure, short write, invalid digest, finalization failure and publication failure. Expected transaction order is checked, Abort always runs and only success invokes completion.
- Additional cases prove declined pending approval never begins storage, and session End during finalization prevents publication. They pass20 fresh race repetitions along with existing storage-boundary ownership cases.
- The actual path transaction rejects Publish before Finalize, writes after Finalize, repeated Finalize/Publish, and a destination collision appearing after staging. Repeated Abort removes private staging while preserving existing/final output; completed mode remains0600.
- The path-transaction baseline passed all five normal CI jobs at1afceb0 in [run44504429692](https://github.com/MaisySen/Mnet/actions/runs/44504429692), after the separately documented source-guard and DNS fixture corrections. No emulator/native/device/provider execution or new pixels are claimed. A local fault adapter proves the receiver contract, not an Android chooser or content provider.

## Next bounded work

Define and test an opaque export capability and explicit completion/cleanup contract before adding any document-provider adapter. Verify exact Android SDK descriptor/stream and cancellation semantics from source/reference, rather than rely on URI-to-path conversion or a guessed provider guarantee. Keep production mobile/viewer reception disabled throughout this preparation. Do not change legacy headless policy, Multies authority, device routes, signed distribution or release behavior.

The next [internal export seam](file-export-capability.md) is now implemented behind a local-only selection, with real-mux fault tests. No production provider or consumer entry point is enabled.
