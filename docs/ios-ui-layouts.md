# iOS shared SwiftUI layout preparation

19 November 2026, Australia/Sydney. Shared-component macOS pixels inspected; full iOS visual acceptance remains open.

The actual iOS ContentView now delegates its visible content to MnetConnectionPanel. The shared production component now uses the pinned Multies default light identity, neutral surfaces and blue source accent, matching the corrected Android/web source treatment. It has a connection card, explicit Connect/Disconnect action, device name, expandable manual settings, a masked auth key, persistent labels and setup/permission limits. Empty server/network settings expand the manual section. ContentView retains preferences, event subscriptions, selected-manager policy, read ownership and the existing explicit connection/disconnection flow. OS status codes now have readable labels rather than numeric strings. No Multies integration is added.

## Render boundary

`sh scripts/render-swiftui-layouts.sh` interprets only the production layout and a synthetic render harness on macOS, using NSHostingView and AppKit bitmap capture. It does not compile/link/install a new host or iOS app, avoiding a host executable's default ad-hoc signature. It does not invoke a simulator, signing tool, provisioning, an account API, preferences, NetworkExtension, provider installation or tunnel startup. The renderer's action closure fails immediately if called. Output is five synthetic-state PNGs in MNET_SWIFTUI_OUTPUT, collected by normal macOS CI as swiftui-component-previews.

The implementation is shared with production, not a separately authored mockup. The renderer is native macOS SwiftUI, not iOS. Actual output must be inspected before claiming even shared-component layout acceptance. macOS text fields/buttons, Dynamic Type handling, text metrics and intrinsic sizing can differ from iOS. Full iOS safe-area/navigation/keyboard/scroll/assistive-technology and device rendering remain unverified. No physical-device or VPN runtime investigation is selected.

## Checks

Linux source-only boundary tests cover the shared component, no-effect renderer imports/inputs and retained manager/event/enrollment safety wiring. Existing macOS general CI typechecks the real app plus component against the iOS simulator SDK, then retains its pre-existing unsigned app/extension compile/link/package check. None of those checks constitutes iOS runtime or visual acceptance. The held TestFlight workflow is not invoked or edited.

The initial ImageRenderer attempt passed execution but failed visual review: native text inputs became yellow unsupported-view markers. Those diagnostic PNGs are not accepted UI previews. The follow-up hosts the same component in NSHostingView and captures the native view hierarchy. The corrected NSHostingView output at dfaa8a5 was downloaded with its GitHub artifact SHA256 verified and all five PNGs inspected. Native fields now render without unsupported-view markers. Requested widths are preserved at320,390 and640 pixels, with no clipping observed in the captured full-height component. This establishes that bounded shared-component hierarchy only, not a completed iOS UI result. macOS Dynamic Type may not scale like iOS; the accessibility-size fixture does not establish an enlarged iOS text result.


## Observed limitations and follow-up

The host automatically selects the device-name text and draws grey actions. Neither proves iOS focus or enabled/disabled appearance. The accessibility-size request does not visibly enlarge macOS text and is not large-text acceptance. Its local fixture name now says accessibility-request rather than large to avoid implying otherwise. The bitmap-width assertion now passes on current-source macOS rendering, rejecting silently unconstrained captures.

The initial corrected component artifact is14488025288 from normal CI run44433394023, SHA25607185d6382f0b1b48ec74df2f8c9f77511615bd595582a5c32f7b0558ea98627. That artifact remains historical. Publication of Mnet engineering docs/previews was subsequently approved; the current-source results below supersede its compiler/render availability limits, not its iOS-runtime limits.


## Local Multies branding correction

The later19November correction replaces the independently chosen treatment with the actual Multies default identity and components. See multies-brand-alignment.md for pinned source provenance, local verification and remaining platform limits. Earlier images above remain historical layout evidence, not the current brand reference. Source was pushed after engineering-docs publication approval; Multies remains unchanged.


## Current-source hosted checkpoint

Normal CI run44477503883 at76978fbccc7631ff9109153aa0ba20dacc14fe29 passed the iOS SDK source check and hosted SwiftUI render. Artifact14507660354, SHA25617b2149e644cec03d5d29029212e8dd0536b9f526fb2f45d0b0cbf00741f2879, contains five current synthetic component images. All five were downloaded and visually inspected: setup390, settings320, connected390, error390/accessibility-request and busy640. Native controls render without unsupported-view markers and the specified widths hold. The source mark and light surfaces are visible; active macOS buttons remain gray, while busy has a distinct disabled treatment. This is not proof of the iOS primary-action color, Dynamic Type, safe areas, keyboard or actual device behavior. No simulator was launched for this visual check.

- [Current compact settings](ui/multies/swiftui-settings-320.png)
- [Current setup](ui/multies/swiftui-setup-390.png)
- [Current connected state](ui/multies/swiftui-connected-390.png)
- [Current error, accessibility size requested, not iOS large-text acceptance](ui/multies/swiftui-error-390-accessibility-request.png)
- [Current busy state](ui/multies/swiftui-busy-640.png)
