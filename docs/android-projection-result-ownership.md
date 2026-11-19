# Android projection-result ownership

19 November 2026, Australia/Sydney. Source and simulated framework evidence only.

## Defect and bounded fix

The hosting Activity previously accepted any successful projection result with its fixed request code, even if there was no current Start request or a prior request had already been declined/consumed. An unrequested or duplicate result could therefore forward the projection Intent to the hosting service again. This is callback ownership, not evidence of an Android permission bypass or an externally exploitable entry point.

The callback now requires the Activity's pending Start flag and clears it before forwarding any success. A valid pending success still starts the existing service request; cancel or missing result data consumes the pending attempt without starting. A different request code does not consume the attempt. Duplicate/unrequested and late-after-cancel results are ignored. Existing auto-accept snapshot/reset, service capture guards, accessibility/control consent and all other connection policy remain unchanged. A real pending request lost to Activity recreation is not silently restored; the user must start again. This small change does not claim unique generation binding across every possible later Android request or installed-process lifecycle.

No layout, platform setting, capture permission, installed app, running host, file destination or consumer reception was changed. No release or Multies change is involved.

## Evidence

Two new tests execute the actual HostingActivity callback on four simulated API levels (24/25/26/33). Eight cases failed against old source: an unrequested or late-after-cancel result enqueued a service start. Restored source passes the full200-case selected framework suite. New cases verify one pending success, duplicate refusal, unrelated-code preservation, denial/empty-data consumption and late-result refusal.

The test explicitly supplies pending state through reflection rather than opening real Android projection consent. Intent/service endpoints, Go and capture remain simulated. This does not prove OS projection-token behavior, installed lifecycle, process death, OEM dialogs or physical screen capture. No new pixels are claimed because visible layout is unchanged.

Existing projection callback-order/release/nonsticky/duplicate-start/frame-gate source/helper checks pass, including100 JVM stop races. API24-36 compatibility/source checks and credential/style/diff checks pass. Initial local setup used the default JDK11, which could not read the pinned framework dependency's JDK17 classes; rerunning with the already available JDK17 produced the stated test failures and passing result. Current normal cross-platform CI is pending; no native/device workflow was started.
