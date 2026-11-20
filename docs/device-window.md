# Read-only native Mnet device window

Linux X11 engineering builds now accept:

```text
mnet [--state /absolute/path/to/state] devices
```

This is an actual native window backed by the selected running daemon's capability-protected local status API, not prototype device data. It belongs to Mnet networking diagnostics, not Remote Desktop or Shared Desk. It cannot enable or disable networking, enroll, select a session recipient, approve access, send input, change a layout or save trust. It has no Multies sign-in or eligibility verifier. Existing engineering enrollment and all consumer release gates remain unchanged.

## Snapshot, not a live-access claim

On open, the window makes one authenticated `GET /localapi/status` through the existing local client. Refresh or R requests another snapshot. Pagination makes no request. There is no background polling. The selected local state directory supplies the existing capability; no token, approval link or server URL is requested or displayed by this window.

The window shows the daemon-reported state, local device/address, network label and snapshot receipt time. Up means reported local readiness, not end-to-end traffic proof. Names and network labels are not verified people, Multies identity, workspace membership or remote-input consent. Peer rows show reported online/offline/unknown state, path category and handshake age at that snapshot. Unknown and future-clock telemetry are not turned into online claims. Endpoint strings, approval URLs, keys, server URLs and raw error text are omitted.

Eight peers fit each page. Previous/Next, or Left/Right, page through every reported row without selecting or contacting a peer. Text is bounded printable ASCII for the fixed X11 font; unsupported characters become `?`. Truncated labels cannot be used for an action because this window offers no peer action.

Refresh immediately clears old readiness and peer rows while its bounded request runs. Repeated Refresh cannot create concurrent requests. Failure or a malformed response leaves no previous successful rows behind. Non-Up states hide peer rows, even if the daemon response contains an older map. Snapshots expire after30 seconds and their rows disappear until a person refreshes. Time labels and fixed explanatory text remain explicit about this being reported snapshot telemetry.

![Actual built native window with a synthetic private-daemon snapshot](ui/devices/window-up.png)

![Actual third page, showing the last three of19 synthetic peers](ui/devices/window-last-page.png)

![Actual Refresh loading state, with previous readiness and peer rows cleared](ui/devices/window-loading.png)

## Keyboard focus and activation

Tab and Shift+Tab now move a visible double-line focus ring among enabled Refresh, Previous, Next and Close controls. Enter or Space acts only when that same key is released on the same still-enabled focused control. Moving focus through Tab, losing local window focus, receiving a snapshot, expiry, or starting a new Refresh cancels a pending activation. A disabled control is skipped, not silently activated later. Existing R, arrow keys, Escape and mouse controls remain available. This adds no peer selection or network/input authority.

![Actual keyboard focus ring on Close, with Next unavailable on the last page](ui/devices/keyboard-focus.png)

The actual built-window acceptance drives Tab/Shift+Tab, Enter/Space pagination and keyboard Refresh/Close. It verifies that pagination sends no API request, an unmatched release cannot close the window, and a held activation cannot transfer through Tab or an independent focused X11 window. The old UI fails the keyboard pagination regression. Thirty model/device race repetitions, the full uncached race/vet/build suite and Darwin/Windows CLI builds pass locally. Final focus, populated and loading pixels were inspected. This is keyboard usability on the supported X11 window, not native accessibility, DPI or screen-reader acceptance.

## Local lifecycle and limits

Close, Escape, normal window-manager close and process termination end this read-only window. Its outstanding request is canceled and joined; it does not stop the daemon or alter its network state. Display/drawing failure or unsupported resize ends the window rather than hiding live controls. A missing local capability fails without contacting the daemon. Status responses are bounded to1MiB and4096 peers; malformed, trailing, missing-state or oversized responses are rejected with fixed local diagnostics. Unknown nonempty daemon states are displayed as unknown, with no peer rows.

The window requires a local Linux X11 display of at least940x540 pixels and the built-in fixed font. Its fixed900x500 layout has44px-high action targets and visibly unavailable Refresh/page controls. Other platforms return an explicit unsupported diagnostic and retain ordinary `mnet status`; no headless or browser fallback starts. Native accessibility, DPI scaling, Windows/macOS UI and physical display acceptance remain work. This is small engineering UI, not the intended installed consumer device/setup experience.

## Acceptance evidence

`python3 scripts/devices-window-validation.py` builds the real race-enabled CLI, starts a private synthetic Unix daemon fixture and opens the actual window on a disposable1024x768 X server. It checks:

- Only authenticated status GETs occur; extra CLI arguments are refused before an API request.
- Nineteen peers are reachable across three pages without API fetches for pagination.
- Down, waiting-approval, request error, malformed JSON, loading and expiry remove previous peer pixels. The driver inspects the actual row-region pixels, not only a title or model state.
- There is no automatic polling during the real31-second expiry wait. Repeated Refresh while waiting is coalesced.
- Escape during an outstanding request promptly cancels and joins the real process. The actual Close button and SIGTERM also end cleanly. A missing capability or absent X11 display sends no request. Unsupported resize ends the process.

Pure tests cover cancellation, bounded decoding, private-field omission, text controls, unknown/future telemetry, pagination and absent owners. Thirty focused race repetitions and full uncached race/vet/build plus Darwin/Windows CLI cross-builds passed locally. The unchanged general session CI job now includes the native private-fixture check and retains only synthetic screenshots/client diagnostics on failure, never the disposable capability state. Hosted results must still be accounted per exact commit.

This fixture proves built UI/local-API behavior. It does not join a live network, validate production Multies identity, prove real peer traffic, grant remote authority or establish physical/mixed-OS readiness.


## Exact mouse gesture ownership

The read-only device window now requires a matching left press and release inside the same enabled44px control. An outside press released over Next, Next pressed then released over Refresh, or an inside press released outside cannot paginate, read the daemon or close the window. This corrects a concrete old-source native regression, not a new peer action.

Pending mouse activation is retired by focus loss, Tab or another keyboard gesture, snapshot refresh, expiry or disabled state. Holding Refresh across a new snapshot or expiry does not turn its later release into a new request. Existing keyboard controls, manual authenticated GET-only Refresh and local-only pagination remain unchanged. No peer is selected and no network, session or enrollment authority is created.

The actual built-window journey reproduces the old mismatch failure, then passes exact/mismatched targets, pending Close through independent focus loss, held Refresh across expiry/new snapshot,19-peer pagination and existing fail-closed lifecycle checks. Thirty focused races, full uncached race/vet/build and Darwin/Windows CLI builds pass. Current populated/expired native pixels were inspected. These remain private synthetic local-daemon fixtures, not live Multies eligibility or physical-device VPN proof.
