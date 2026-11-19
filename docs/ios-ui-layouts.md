# iOS shared SwiftUI layout preparation

19 November 2026, Australia/Sydney. Shared-component macOS pixels inspected; full iOS visual acceptance remains open.

The actual iOS ContentView now delegates its visible content to MnetConnectionPanel. The shared production component uses the same gray-green/white/teal treatment as Android and the control-service web UI. It has a connection card, explicit Connect/Disconnect action, device name, expandable manual settings, a masked auth key, persistent labels and setup/permission limits. Empty server/network settings expand the manual section. ContentView retains preferences, event subscriptions, selected-manager policy, read ownership and the existing explicit connection/disconnection flow. OS status codes now have readable labels rather than numeric strings. No Multies integration is added.

## Render boundary

`sh scripts/render-swiftui-layouts.sh` interprets only the production layout and a synthetic render harness on macOS, using NSHostingView and AppKit bitmap capture. It does not compile/link/install a new host or iOS app, avoiding a host executable's default ad-hoc signature. It does not invoke a simulator, signing tool, provisioning, an account API, preferences, NetworkExtension, provider installation or tunnel startup. The renderer's action closure fails immediately if called. Output is five synthetic-state PNGs in MNET_SWIFTUI_OUTPUT, collected by normal macOS CI as swiftui-component-previews.

The implementation is shared with production, not a separately authored mockup. The renderer is native macOS SwiftUI, not iOS. Actual output must be inspected before claiming even shared-component layout acceptance. macOS text fields/buttons, Dynamic Type handling, text metrics and intrinsic sizing can differ from iOS. Full iOS safe-area/navigation/keyboard/scroll/assistive-technology and device rendering remain unverified. No physical-device or VPN runtime investigation is selected.

## Checks

Linux source-only boundary tests cover the shared component, no-effect renderer imports/inputs and retained manager/event/enrollment safety wiring. Existing macOS general CI typechecks the real app plus component against the iOS simulator SDK, then retains its pre-existing unsigned app/extension compile/link/package check. None of those checks constitutes iOS runtime or visual acceptance. The held TestFlight workflow is not invoked or edited.

The initial ImageRenderer attempt passed execution but failed visual review: native text inputs became yellow unsupported-view markers. Those diagnostic PNGs are not accepted UI previews. The follow-up hosts the same component in NSHostingView and captures the native view hierarchy. The corrected NSHostingView output at dfaa8a5 was downloaded with its GitHub artifact SHA256 verified and all five PNGs inspected. Native fields now render without unsupported-view markers. Requested widths are preserved at320,390 and640 pixels, with no clipping observed in the captured full-height component. This establishes that bounded shared-component hierarchy only, not a completed iOS UI result. macOS Dynamic Type may not scale like iOS; the accessibility-size fixture does not establish an enlarged iOS text result.


## Observed limitations and follow-up

The host automatically selects the device-name text and draws grey actions. Neither proves iOS focus or enabled/disabled appearance. The accessibility-size request does not visibly enlarge macOS text and is not large-text acceptance. Its local fixture name now says accessibility-request rather than large to avoid implying otherwise. A bitmap-width assertion is prepared locally to reject silently unconstrained captures in future runs; that revision has not run on macOS or been pushed.

The initial corrected component artifact is14488025288 from normal CI run44433394023, SHA25607185d6382f0b1b48ec74df2f8c9f77511615bd595582a5c32f7b0558ea98627. Pixels remain CI artifacts; no SwiftUI preview was added to the docs website. New pushes are paused pending the owner's decision on existing automatic public docs publication. Local revisions are not deployed results.


## Local Multies branding correction

The later19November correction replaces the independently chosen treatment with the actual Multies default identity and components. See multies-brand-alignment.md for pinned source provenance, local verification and remaining platform limits. Earlier images above remain historical layout evidence, not the current brand reference. New source and pixels remain local while public-documentation publishing is undecided.
