# Disabled Android document-selection ownership

19 November 2026, Australia/Sydney. Native routing source and simulated framework evidence only. No consumer reception or system picker is enabled.

## What is implemented

`DocumentSaveActivity` is deliberately absent from AndroidManifest and has no app/service/Go launcher. Its package-private preparation API binds an immutable local offer snapshot to one opaque in-memory request and one Activity instance. The Activity's real lifecycle methods build an `ACTION_CREATE_DOCUMENT` Intent, consume a returned content URI once and hand it to the existing unopened [Java-owned output adapter](android-document-export-preparation.md).

This is not yet a Go approval bridge. The immutable snapshot names the session, request, stream, filename, byte count and SHA256. A future bridge must obtain those fields from the exact current `FileApprovalRequest`, calculate its remaining lifetime, validate its Done/current authorization state and return the unmodified original request to Go. Java fields are not proof of authorization. A result must never be matched by filename, URI text or shared request code alone.

The bounded native request provides:

- At most eight pending or selected-but-unreleased handles. A selected output continues holding its slot, rather than allowing an unbounded list of live grant owners.
- One explicit launch Intent carrying only a random local lookup handle, not a peer-provided destination or serialized approval. A second launch or second Activity attachment cannot reuse it.
- One picker launch per Activity instance. Request code1 is local to that one instance and is not shared with projection or reused for later offers in it. A duplicate/wrong-code result cannot replace its request.
- A monotonic selection deadline of at most20seconds. This is a maximum, not permission to reset or extend the Go deadline. Expiry/cancel/destroy retires the request and refuses later results. A callback after elapsed expiry is rejected even if the main-loop expiry callback has not run yet.
- No saved approval or registry restoration. A recreated Activity with saved state finishes without launching or approving; an unknown/retired handle also finishes. A new process has no in-memory request to recover. This is fail-closed source behavior, not installed process-death acceptance.
- An unopened DocumentExport on successful selection. Selecting a URI does not open a provider, start receiving bytes or verify their digest. Future Go staging must still verify bytes before opening the output.
- A retained local handle for the session owner to cancel after Go rejects a stale decision, session End or other failure, and release after successful export. Owner cancellation closes an opened output or prevents a later open. Terminal answer callbacks are cleared so a completed selection does not keep an old UI callback alive.

## Temporary permission lifetime changes the handoff

The general Android storage guide describes non-persisted access in broad until-restart terms. The official storage sample and Intent/FileProvider guidance give a narrower lifetime warning: a temporary URI grant can depend on the receiving Activity/task, and extending it to another component requires the URI and grant flags in the receiving Intent's data or ClipData. A plain Java callback does not establish such a grant transfer.

The first local candidate finished its Activity immediately after returning an unopened output. That was rejected during source review before publication. The current candidate keeps its Activity alive until the local request is released/canceled or the Activity is destroyed. Destruction cancels the output, including an unopened one. No persistable permission is taken and no service grant is synthesized. This conservative lifetime ownership is not proof that any real provider grants access as expected.

After selection there is no new fixed receive-duration timeout: network staging can legitimately outlive the original selection deadline. The bounded slot and Activity remain owned by the retained session handle until explicit release/destruction. A future service must supply reliable terminal release on every outcome before enabling this route. It must also supply appropriate visible waiting/export/cancel UX. The prepared Activity has no consumer progress layout and must not be registered as a finished user journey.

## Framework evidence

With JDK17 and Maven:

```sh
MVN=/path/to/mvn sh scripts/test-android-framework.sh
MVN=/path/to/mvn sh scripts/test-android-framework.sh vpn
python3 scripts/check-disabled-document-save.py
```

All312 selected framework cases pass locally, including60 new document-selection cases,15 on each of APIs24/25/26/33. These exercise the actual Activity lifecycle methods and Intent construction with the existing fake provider, not a mock state-machine-only implementation. They cover exact callback/request identity, one launch/attachment, wrong code/duplicates, two identical offers, cancellation and expiry, empty/denied/non-content results, saved-state recreation, unavailable handles, eight-slot recovery, selection-held capacity, owner cleanup before/after handoff and a failing owner callback. No provider opens before a caller explicitly opens the returned adapter. Removing the single-attachment check causes four duplicate-Activity cases to fail; the accepted source was restored. The separate78-case VPN simulation and API compatibility guards also pass.

The disabled-route guard parses the actual manifest and checks that other production Java files have no document picker/output caller. Its built-in mutations reject a manifest registration or new caller. It runs at the start of the framework script. These are source checks, not enforcement against reflection, arbitrary native code or an installed package. Enabling the route later requires an explicit review and intentional guard change.

The initial duplicate-Activity test read Robolectric's shared started-Intent queue without draining the first Activity's launch. Fixing that test bookkeeping produced the stated passing result. The later deliberate attachment-check mutation is a separate test-sensitivity check, not a platform failure. No system picker, real provider, Android grant enforcement, actual process kill/restart, JNI or physical device runs here. No layout pixels are claimed for this unregistered routing component.

## Still not decided or enabled

The existing20second local approval /30second sender acceptance window is not a usable guarantee that every person can finish a document picker. A late picker can create an empty document even when its request has expired. This implementation refuses that late result; it does not delete the document, extend the wire deadline or attach it to a retry. A preselected destination/retry or protocol waiting design and clear cancellation/recovery wording are still required before enabling reception.

The exact native-to-Go bridge, service/session teardown, real Activity/task grant lifetime, OS revocation, provider crash/process death, actual pipes, partial-document recovery and timeout UX remain gates. No Multies, existing host/headless defaults, app permission/manifest, Apple, signing, release or production service changes are included. Current checkpoint normal CI is pending until reported separately.

## Source grounding

Read19November2026:

- [Android activity-result guide](https://developer.android.com/training/basics/intents/result): Activity/process can disappear while an external result is pending; additional request state needs its own handling.
- [Activity source, android-14.0.0_r1](https://android.googlesource.com/platform/frameworks/base/+/android-14.0.0_r1/core/java/android/app/Activity.java): per-Activity result routing and lifecycle APIs, not a unique-offer authorization contract.
- [Android Activity lifecycle](https://developer.android.com/guide/components/activities/activity-lifecycle): destruction/recreation and saved-instance behavior.
- [Android storage guide](https://developer.android.com/training/data-storage/shared/documents-files): create-document behavior, optional persisted permission and invalidated access after move/delete.
- [Official Android storage sample](https://github.com/android/storage-samples/blob/main/ActionOpenDocument/app/src/main/java/com/example/android/actionopendocument/MainActivity.kt): temporary permission lifetime warning and grant propagation through Intent data/ClipData.
- [FileProvider permission guidance](https://developer.android.com/reference/androidx/core/content/FileProvider): temporary Activity/task/service grant lifetime and extension to app components.
- [Intent permission reference](https://developer.android.com/reference/android/content/Intent#FLAG_GRANT_WRITE_URI_PERMISSION): write-grant flag semantics. This prepared route does not take or persist a grant itself.
