# iOS packet-tunnel run ownership

Before this change the provider stored a mutable optional Node, launched asynchronous Prepare/settings/start callbacks and used reusable Stop on shutdown. Prepare/settings failures did not dispose the node. A stop during pending preparation/settings could leave a callback starting its captured old node later, even if the provider's mutable field had changed. These are source findings, not physical-device incident reports.

The provider now uses a one-run coordinator. Each run owns its resource, start completion, in-flight stage and permanent Close. State transitions occur on a serial queue; potentially blocking Go prepare/start/close work runs off that queue. Stop and failure mark the run terminal before closing its own node. A pending start completes once with cancellation/error. Late preparation/settings callbacks cannot dispatch new activation; already-dispatched activation may finish, but permanent Node.Close prevents reuse and remains responsible for cleanup. Duplicate settings callbacks are consumed once.

The retiring run remains owned until both Close and its in-flight stage finish. A replacement start is refused while the prior run drains; it is not silently queued or allowed to overlap old network settings. Stop completion waits for that drain. If a platform settings callback never arrives or native Close/work never returns, completion can remain pending. This change does not invent a timeout or promise a hard stop latency. That OS/runtime behavior remains an acceptance gate, rather than letting a stale settings callback overwrite a new run.

Creation, exit selection, Prepare, settings and Start errors are no longer swallowed. Exit selection is applied even when empty so it matches this requested configuration rather than retaining a stale saved choice. Creation failure has no resource; every subsequent failure permanently disposes the created node. The generic coordinator tests inject resources/stages, never real preferences, VPN packets, credentials or Apple accounts.

## Validation scope

Three Python wiring checks pass locally. New Swift host checks exercise success/replacement, duplicate settings callback, stop during blocked preparation/settings/activation, overlap refusal and four setup-failure boundaries with exact Close ownership. The macOS general CI step must execute these and typecheck the actual full PacketTunnelProvider plus Flow against freshly generated current Objective-C headers and the iOS SDK before compiler success is claimed. No linked Go framework, signed extension, simulator launch, entitlement acceptance or physical iOS runtime is implied.

There is no UI layout change. Existing manual enrollment, credential storage, default-route policy, consumer Multies setup, runtime network recovery and signed delivery remain separate unfinished work. No Apple signing/account/device, production service, paid resource, release artifact or held native Android investigation is changed.


First CI execution passed the host ownership scenarios and four other general jobs, then the new full-provider SDK check found that Prepare's generated nonnull String result retains an explicit NSErrorPointer in Swift instead of importing as throws. The provider now checks that error explicitly before settings creation. This compile-surface correction does not waive the error or change lifecycle policy. The changed-revision full-provider compiler result must be collected again.
