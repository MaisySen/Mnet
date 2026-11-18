# iOS app enrollment-input lifetime

The engineering join form previously loaded/saved its auth key in ordinary UserDefaults alongside server/name settings, on both Connect and Disconnect. The app no longer reads or writes that duplicate preference. It starts with an empty in-memory form value, removes the legacy preference on appearance/save without reading it, and passes an entered key only as part of this explicit Connect request's options. After startVPNTunnel accepts that handoff, the SwiftUI form value clears. If a synchronous configuration/start error occurs, the entered value remains available for retry during the current app lifetime. An asynchronous join failure after handoff may require entering the key again.

The app's non-secret server/network/device/exit preferences are preserved. Cleanup is idempotent and does not remove other preferences. Users with an uncompleted enrollment who relied on the old app preference must re-enter their key. No real preference domain or credential is inspected or changed during this source/test work; Swift tests use a unique disposable suite and synthetic values.

## Important retention boundary

This does not eliminate all on-disk enrollment-key retention. The existing native daemon deliberately stores AuthKey beside its private identity, in protected private state, to support the legacy server-forgotten rejoin path. That policy remains unchanged. Existing Android engineering preferences also remain unchanged. Clearing the app's Swift String or deleting a preference is not guaranteed secure memory/disk erasure, backup deletion, remote key revocation or a retention-policy decision. The native extension necessarily receives the connect options; in-flight copies may remain until those operations finish. Consumer Multies enrollment and the eventual credential/revocation/retention model are separate held integration work.

## Verification scope

Three local Python source checks verify no app preference key read/write, exact non-secret helper wiring and clear-after-successful-handoff ordering. New Swift tests use an isolated preferences suite to verify legacy-copy removal, idempotence and unrelated/non-secret settings preservation. Exact-revision macOS CI must execute those tests and compile/link the app before that evidence is claimed. The existing unsigned linked-build boundary remains: no launch, real profile, signing, entitlement registration, account, device or release. There is no UI layout change or masked-entry visual acceptance in this unit.
