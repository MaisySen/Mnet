# iOS shared SwiftUI layout preparation

19 November 2026, Australia/Sydney. Visual acceptance is pending actual renderer output and inspection.

The actual iOS ContentView now delegates its visible content to MnetConnectionPanel. The shared production component uses the same gray-green/white/teal treatment as Android and the control-service web UI. It has a connection card, explicit Connect/Disconnect action, device name, expandable manual settings, a masked auth key, persistent labels and setup/permission limits. Empty server/network settings expand the manual section. ContentView retains preferences, event subscriptions, selected-manager policy, read ownership and the existing explicit connection/disconnection flow. OS status codes now have readable labels rather than numeric strings. No Multies integration is added.

## Render boundary

`sh scripts/render-swiftui-layouts.sh` interprets only the production layout and a synthetic render harness on macOS, using NSHostingView and AppKit bitmap capture. It does not compile/link/install a new host or iOS app, avoiding a host executable's default ad-hoc signature. It does not invoke a simulator, signing tool, provisioning, an account API, preferences, NetworkExtension, provider installation or tunnel startup. The renderer's action closure fails immediately if called. Output is five synthetic-state PNGs in MNET_SWIFTUI_OUTPUT, collected by normal macOS CI as swiftui-component-previews.

The implementation is shared with production, not a separately authored mockup. The renderer is native macOS SwiftUI, not iOS. Actual output must be inspected before claiming even shared-component layout acceptance. macOS text fields/buttons, Dynamic Type handling, text metrics and intrinsic sizing can differ from iOS. Full iOS safe-area/navigation/keyboard/scroll/assistive-technology and device rendering remain unverified. No physical-device or VPN runtime investigation is selected.

## Checks

Linux source-only boundary tests cover the shared component, no-effect renderer imports/inputs and retained manager/event/enrollment safety wiring. Existing macOS general CI typechecks the real app plus component against the iOS simulator SDK, then retains its pre-existing unsigned app/extension compile/link/package check. None of those checks constitutes iOS runtime or visual acceptance. The held TestFlight workflow is not invoked or edited.

The initial ImageRenderer attempt passed execution but failed visual review: native text inputs became yellow unsupported-view markers. Those diagnostic PNGs are not accepted UI previews. The follow-up hosts the same component in NSHostingView and captures the native view hierarchy. Until the corrected output is available and inspected, this is implemented source and a bounded feasibility path, not a completed iOS UI result. macOS Dynamic Type may not scale like iOS; the accessibility-size fixture does not establish an enlarged iOS text result.
