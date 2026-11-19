# Consumer journey prototype

This is an isolated **simulation**, not a connected Mnet client. It uses sample people, a sample workspace and sample devices. It makes no external requests, collects no credentials, uses no device APIs and stores no state. Nothing here proves a working Multies integration, VPN, screen capture or input control.

## Run locally

From the repository root:

```sh
python3 -m http.server 8080 --directory prototype --bind 127.0.0.1
```

Open `http://127.0.0.1:8080`. Stop the server when finished. Do not expose it on a public interface.

## Review

The scenario picker covers owner setup, existing devices, ineligible login, denied permission, failed connection, offline target, declined session and revoked membership. All real-looking actions are simulated. The first complete journey is sign-in handoff, workspace choice, VPN explanation, separate connection checks, device details, attended viewing, separate control consent, interruption, safe stop and removal. File transfer, recording, unattended access and a real OS permission dialog are outside this prototype.

Normal setup has no server/network/key fields. A local Up status does not complete setup: identity, private DNS and actual peer traffic must all pass. Session viewing and input control are separate permissions. A cancelled request cannot accept a late response; membership removal ends the simulated session; reconnect does not silently restore control. Removing a device and signing out are distinct actions.

The frame shows a proposed Multies Devices entry point using simple responsive controls. It is not a copy of the complete Multies shell and does not modify that repository. Real integration must use the existing account/workspace authority and shared navigation model. A valid upstream identity-provider subject alone is insufficient. An authorized, narrowly scoped Multies-to-Mnet bridge is still required before production integration.

## Verify

```sh
node --test prototype/model.test.mjs
node prototype/verify.mjs
# Optional private screenshots outside the repository:
SCREENSHOT_DIR=/tmp/mnet-prototype-screens node prototype/verify.mjs
```

Node22+ and Chrome are needed for browser verification; `CHROME_BIN` can select the Chrome binary. The browser runs against an ephemeral loopback server with a fresh disposable profile, not the user's browser. Verification clicks28states at320,390and1280pixels, asserts expected headings, no horizontal overflow,44px minimum button height, separated review controls, heading focus and no browser exceptions. State-model tests explore all reachable fixture states and assert permission invariants. These are prototype tests, not server-security or physical-device proof.

Visual inspection found a320px scenario-picker overlap and corrected it before reporting. Mobile flow screenshots were inspected for wording, clipping, hierarchy and separation of view/control consent. Desktop welcome/session views were also inspected. The entire final width/state matrix is captured by the browser verifier; only representative widths/states have a separate human pixel review.

## Source-brand follow-through, 19 November 2026

The isolated prototype now uses the same pinned Multies mark, neutral light palette, blue primary/hover, neutral secondary buttons and focus treatment as the actual Mnet screen correction. Its proposed sidebar is still not a copy of the full Multies shell. Sample-only warnings stay above every screen and in the footer; no model transition, network/sign-in integration or device capability was added. All84 screen/width checks and15 state-model tests pass after the style change. Representative390px welcome,320px separate control consent and1280px active-view pixels were inspected; source-palette and supported-light/forced-colors checks are included. This is prototype design evidence, never product authorization or native consent proof. Existing dark canvas is a simulated screen, not a new Multies dark theme.

Scrolled-action review found the next focused heading could stay outside the viewport after a long form/action. Transitions now reveal that heading. Verification scrolls each action into view before clicking and asserts heading visibility for all84 cases, with declined-permission and ended-session viewport pixels separately inspected. Full-page export follows the viewport capture because it can change capture scroll state. These are simulated navigation checks, not native permission behavior.
