# iOS connection-status read ownership

The app originally refreshed only on appearance and immediately after Connect/Disconnect. A later OS status or Settings change could leave its action based on old state. Overlapping preference-load callbacks had no generation, so an earlier read could overwrite a newer result or explicit-action error. The connection's raw VPN status remains a local OS status, not mesh traffic proof.

The app now refreshes on VPN-status/configuration notifications and foreground activation, without a timer or polling loop. Each read gets a generation ticket; only the latest requested read may update state. Connect/Disconnect and disappearance invalidate outstanding reads. Events during an explicit save/reload/start operation are deferred to that operation's final refresh instead of racing its error. The source does not install background monitoring, start a tunnel from a notification or mutate configuration during refresh.

Each Connect now snapshots all form options at the explicit tap, before asynchronous manager lookup/save/reload. Editing the form while that work waits cannot silently change that already requested destination/key. Successful handoff clears the form's key only if it still equals the submitted value, so newer user input is not erased by an old completion. Existing ordinary-preference secret removal and native protected-state retention limits are unchanged.

## Evidence scope

Three local event-wiring checks cover notification/foreground subscription, no polling, read-generation ownership, action invalidation and immutable-options ordering. Swift host tests cover latest read, action/disappearance invalidation and100concurrent ticket allocations. Those do not simulate system callbacks or render the UI. Exact-revision CI must execute the Swift tests, iOS SDK checks and current unsigned linked app/tunnel build before those results are claimed.

No UI layout is changed or rendered-pixel acceptance claimed. Actual SwiftUI subscription lifetime, system Settings transitions, profile errors, signed entitlement/runtime VPN behavior and physical recovery remain separate acceptance work. No real preferences, OS VPN configuration, credentials, simulator launch, device, account, signing or release is used by this unit.
