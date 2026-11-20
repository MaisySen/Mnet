# Desktop entry and complete journey plan

This is a local Linux X11 engineering entry, not the installed consumer app. Multies sign-in, workspace eligibility, enrollment and Shared Desk pairing are not connected here. No successful prototype, current network peer or layout file substitutes for those decisions.

## Current working entry

```
mnet open
mnet open --layout chosen-layout.json
```

![Actual local engineering home](ui/desktop-home/home.png)

The home has three distinct tool choices. Networking opens the actual authenticated read-only device snapshot. Remote Desktop opens the existing current-peer chooser with separate View Only and Pointer View choices. Shared Desk opens only the supplied read-only arrangement inspector; without a supplied layout it is visibly unavailable. No raw-input run or pairing can start from this home.

Opening the home reads no daemon state and starts no network/session operation. A selected home closes and joins its event reader before its child opens. Closing the child returns to a fresh home, not another session. A later device choice rereads current evidence and requires fresh host decisions. Signals cancel the current owner. Existing explicit CLI entry points retain their behavior.

This window uses larger headings/action text and short permission summaries. Technical setup and platform details belong in the owning guides rather than competing with the primary action. The current home uses embedded licensed proportional Go fonts. Supported local prerequisites are Linux X11, a24-bit TrueColor display with the supported32-bit little-endian image format and at least940x600pixels. Other child windows still have their own prerequisites. Unsupported configurations fail; there is no Wayland/native fallback or misleading success screen.

## Readable device choice and honest loading

The real peer chooser uses larger heading, device-row and mode-action text while retaining the complete bounded selected label/IP and exact key in its detail panel. Empty or expired rows do not gain eligibility from the new visual style. The viewer has a central waiting-for-host or waiting-for-first-screen panel rather than an unexplained empty canvas. End stays visible; the panel is removed by actual rendered pixels, never merely by host approval. These are implemented screens, not mockups.

![Actual selected device](ui/desktop-home/selected-device.png)

![Actual admitted first-screen wait](ui/desktop-home/waiting-screen.png)

The first local loading-panel attempt panicked before host admission because its explicit font-name length was wrong. Actual journey checks caught it, and the source now computes that length. Pixel inspection then caught an inherited text-background color; the drawing resets it before repainting the header. Neither intermediate version was committed or published. These failures are separate from the older unresolved blank-canvas fixtures.

## First-screen failure is a real session outcome

Host approval is not proof that pixels are arriving. After admission, the viewer waits at most15seconds for a usable complete screen. Pending host approval has no new timer. A first complete rendered frame disarms the bound, including when the desktop then stays unchanged. A silent or unusable initial stream ends the attempt, closes the viewer/transport and returns to a clear first-screen failure state when launched from home. No automatic reconnect or reuse of host approval occurs.

![Actual first-screen failure return](ui/desktop-home/no-first-screen.png)

This bounds indefinite waiting. It does not diagnose the earlier two concurrent blank-canvas fixture failures. Later unchanged concurrent runs returned first-full captures in100ms-to1s and passed, but those passes are separate evidence. Hosted Chrome first-CDP startup failures are also distinct and remain unresolved.

## Consumer journey, before more surface expansion

The intended installed journey needs these real state transitions and visual priorities:

| Stage | Primary information and action | What must actually happen |
| --- | --- | --- |
| First run | Product name, one short setup explanation, Continue with Multies. | The production bridge must exist before this action can work. Until then show setup unavailable, never a simulated signed-in state or manual network-key fallback as consumer setup. |
| Workspace and device | Current workspace, this device, relevant permission decisions. | Bind fresh verified person/workspace/device/capability evidence. Declining an optional permission must not disable unrelated capabilities. |
| Tool selection | Mnet networking, Remote Desktop, Shared Desk, clearly separate. | Each tool checks its own current scope. A device being online grants no screen or input access. |
| Device selection | Eligible device name, current availability, exact intended mode. | Load real scoped current devices, erase stale selection, revalidate the exact selected key before connecting. |
| Host approval | Who is asking, view-only scope, Allow or Decline. | No pixels before host-local view approval. Pointer control requires its own later host answer. |
| Screen loading | Waiting for the first screen, always-visible End. | Bound absent first pixels after admission; distinguish failure from a working session. |
| Active session | Actual remote pixels, current narrow control state, prominent End. | Local controls never become remote input. Revoke returns to viewing; End retires transport/capture/input rather than only the label. |
| End or failure | Specific outcome and a deliberate return or retry. | Join old owners, discard selection/approvals and require a fresh explicit journey. No background auto-reconnect. |

The engineering home is a partial entry improvement within this plan. Real Multies integration, installed first-run setup, persistent trusted Shared Desk pairing and physical mixed-platform acceptance remain critical-path work. Multies stays read-only, and signing, release, accounts, production infrastructure and additional devices remain outside current execution authority. See [product scope](product-scope.md), [three-tool map](three-tools.md) and [authorization boundary](multies-boundary.md).

## Actual acceptance and limits

`home-validation.py` runs the built executable against a private Unix API and disposable X display. It checks zero home API reads, disabled layout, real device/chooser/inspector routing, child return, no implicit retry, pending-key cancellation and SIGTERM cleanup. No fake home UI replaces the executable.

`MNET_VIEWER_HOME=1 native-pointer-validation.py` takes the full actual home to chooser to host-approved view to separately approved pointer journey. It retains exact two native clicks, one scroll, zero native keys, all three native owners joined, decline/revoke/focus-loss behavior and End returning to a fresh home without surviving host/view windows. Both host sizes remain required.

`first-screen-validation.py` uses actual host approval with a deliberately silent capturer. It checks the production15second bound, native viewer/host closure and the inspected fresh-home failure state. Focused tests separately cover a silent admitted transport, no timeout on pending host consent, owner cleanup and disarming after the first frame on an unchanged desktop. These are same-computer disposable-X11 tests, not physical two-computer or live Multies evidence.

The general CI test steps include these journeys and retain failed synthetic screenshots/logs. Workflow triggers, all non-general native lanes and their path filters are unchanged. Source publication does not close any release gate.

## Installed engineering entry

The [private Linux source installation](linux-local-installation.md) now builds both executables into a new owned prefix and supplies an actual desktop entry. Its acceptance launches that entry through Gio from a special-character path, verifies the installed executable owns the actual home, closes it and removes only unchanged installation files. It does not install a service, start a daemon, enroll a device, alter Multies or create Mnet state. This is a working engineering install/launch/removal path, not the missing production first-run bridge.

An installed Remote Desktop attempt now reports missing local setup, an unreachable daemon, refused local capability and a disconnected local device separately. Fixed guidance does not expose the transport error, server body, key or local path. No peer/mode becomes selectable in these states. The installed acceptance uses the real installed daemon in an explicitly isolated in-memory-TUN down state, stops it, and checks fresh recovery without mutating its saved state. See [installed recovery evidence](linux-local-installation.md#missing-setup-is-now-a-distinct-recoverable-state). This does not invent a Multies sign-in path.

## Original hosted prerequisite failure

Original4b3 failed its first hosted home launch with `home readable fonts unavailable`. Its session job did not reach the new complete home or silent first-screen checks. The newer source declares `xfonts-base` explicitly and adds `home-font-validation.py`: a real built-ins-only X server must reproduce refusal and a server with the declared fonts must open the real home. The requirement is not relaxed. That local correction needs its own hosted run; earlier original failure remains recorded in [STATUS](STATUS.md).


## Branded local home, later isolated branch

The ordinary local entry now uses the same embedded proportional-font visual
style as the opt-in Multies preview. It still routes only the existing three
local tools. No production origin, sign-in bridge, network enrollment, saved
pairing or remote permission is inferred. Shared Desk remains disabled without
an explicit offline layout file. The title and route coordinates remain stable.

The home uploads complete strips into an offscreen pixmap and installs the
completed pixmap as the background before exposing the window. An initial
attempt copied pixels while the window was unmapped; X11 map cleared them and
the real screenshots were blank. Actual pixel inspection rejected that attempt.
The completed-background fix removes the initial blank canvas without a sleep
or test-deadline change. Layout/notice fit tests cover every route/focus state.

The font check intentionally now confirms actual home rendering and exact close
on both built-ins-only and normal-font X11 servers. This is a later embedded-font
implementation, not a repair claim for original4b3 or a change to its historical
missing-font result. Other native child windows still use their existing core
fonts, so their installer/CI prerequisites have not been removed.
