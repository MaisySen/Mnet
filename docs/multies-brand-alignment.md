# Multies brand alignment and UI recovery checkpoint

19 November 2026, Australia/Sydney. Follow-up to the request to use Multies branding in Mnet. This engineering checkpoint contains implementation details and synthetic screen previews, not an app release, Multies integration or production deployment. Publication of these Mnet docs and previews was approved before this checkpoint was pushed. Multies remains unchanged.

## Latest verification summary

The entries below preserve the sequence of findings and fixes. The latest local Android checkpoint is 192 selected framework simulations across API24/25/26/33, plus ten actual-resource raster cases and 90 synthetic images. These are not installed-device, keyboard, TalkBack, launcher, VPN or remote-control acceptance. Current operator/member Chrome regressions, control race/vet and brand checks pass. Fresh full-repository race/vet passed on the final pushed source. Corrected Swift branding now passes current-source iOS SDK source checks and macOS hosted rendering, with all five images inspected. macOS gray active buttons and unscaled accessibility-size requests do not establish iOS control color or Dynamic Type; installed iOS runtime acceptance stays open. See ios-ui-layouts.md for the current artifact and pixel links.

Latest inspected evidence:

- [Android approval action, enlarged](ui/multies/320-large-approval.png)
- [Android approval details destination, enlarged](ui/multies/320-large-approval-details-jump.png)
- [Android error details destination, enlarged](ui/multies/320-large-error-details-jump.png)
- [Member device records, enlarged](ui/multies/account-records-320-large.png)
- [Member unconfirmed sign-out recovery](ui/multies/account-signout-unconfirmed-390.png)
- [Operator device records](ui/multies/operator-device-record-390.png)
- [Operator device records, enlarged](ui/multies/operator-device-record-320-large.png)

## Source of truth

Read-only inspection of the actual Multies repository at8a0e6e2655363ec392c42922fea27e4a7b779a3f, rechecked against the live default-branch HEAD. The brand guide, generated logo assets, mirrored token JSON, implemented web CSS, mobile theme, default branding preset and actual web/mobile Button components agree on a neutral system-font visual language and blue default accent. Asset hashes and exact observed source references are retained in brand/multies/provenance.json. No Multies source, account, policy or deployment was changed.

The guide's short prose mentions blue#0071E3, while its token file and current default light CSS use#0066CC with#0058B0 pressed. The dark palette uses#0071E3. This local light-screen pass follows the implemented light values, not an inferred workspace theme. The separately flagged design pass is off unless configured; its live deployment/flag status was not read, so that treatment is not silently selected.

## Implemented treatment

- Exact rounded M and orange spark geometry, with original proportions/line caps/joins and clear space. The mark-only asset is supported by the source guide; Mnet remains the product name, not a redrawn Multies wordmark.
- Ink#1D1D1F, muted#5A5A5F, canvas#F5F5F7, white surfaces, translucent source dividers, fill#E8E8ED, blue primary actions. Secondary controls use neutral fill and ink, matching the implemented Multies Button rather than making every action blue. Orange#FF6A1A is confined to the logo spark.
- System-first fonts and at least44px targets. Web uses10px control/16px card corners from its component/token source. Native follows the more specific implemented mobile theme:12px controls,18px cards,#F2F2F7 canvas and#5F5F64 muted text, rather than incorrectly forcing web-only measurements onto the native companion. Android fields use the mobile component's outlined white/12px treatment,16px input text and focused blue outline, replacing the generic underlined platform field. Native cards use the mobile16px padding and light elevation. Existing Mnet hierarchy, explicit consent/host controls, manual-development limits, member/operator separation and approval-versus-connectivity wording remain. No legal/provider identity text changed.
- Android source uses the original Multies launcher tile and monochrome notification artwork without edits. These assets have been inspected and resource-packaged, not installed on a launcher or verified as an actual OS notification icon.
- The production shared SwiftUI component uses matching tokens and source mark geometry. New compiler/render output is still unavailable locally. Earlier macOS previews predate this correction and do not prove corrected branding. No iOS Dynamic Type, keyboard, safe-area or enabled/disabled appearance claim is made.

This is the default light brand treatment, not implementation or acceptance of Multies workspace-specific branding or complete dark-mode behavior. It does not add Multies sign-in, enrollment or a device registry.

## Verification

Fresh full uncached Go race suite, go vet, real Chrome account/local provider and operator regressions passed. Five brand/provenance tests pass;18 existing iOS source-boundary checks pass. Native Android actual-resource/API33 raster fixtures pass9/9 and produce57 synthetic state/viewport PNGs. Selected Activity/service/helper framework tests pass168/168 and VPN-service simulation passes78/78. Those simulations use fakes, not live VPN, Go/JNI, Accessibility or physical devices.

The first parallel validation batch exceeded its command time limit while the selected framework suite was still running, so that suite was rerun independently. It exposed four viewer gesture-test failures: the added brand header moved the picture center below the scroll viewport, causing the test to tap outside the picture. The test now scrolls the actual nested viewport to the picture, asserts the target is visible, dispatches the tap, then proves a subsequent drag scrolls farther without becoming remote input. All168 cases pass after that fixture correction; input gating/transport logic was not changed.

Visually inspected corrected local Android home/setup/enlarged setup, active sharing/enlarged lower controls, pending viewer/enlarged chat/landscape paused state, plus browser landing/member/recovery/device approval/operator/policy views. Inputs, wrapping, fixed footer and action ordering remain readable in inspected views. Native captures are actual production Views/resources with synthetic state; browser captures are actual local Go/store/provider fixtures. Neither is a real remote screen or deployed customer account.

## Reproduce

Run scripts/test-multies-brand.py and existing Android/browser layout regressions as documented in android-ui-layouts.md and web-ui-layouts.md. The SwiftUI render route remains documented separately in ios-ui-layouts.md. Corrected-brand source/evidence must not be called a passed macOS or iOS checkpoint until an authorized compiler/render route checks it.


## Selected local evidence

- [android-home.png](ui/multies/android-home.png)
- [android-active-sharing.png](ui/multies/android-active-sharing.png)
- [android-settings-large-bottom.png](ui/multies/android-settings-large-bottom.png)
- [android-host-large-bottom.png](ui/multies/android-host-large-bottom.png)
- [android-viewer-pending.png](ui/multies/android-viewer-pending.png)
- [android-viewer-chat-large.png](ui/multies/android-viewer-chat-large.png)
- [android-viewer-paused-landscape.png](ui/multies/android-viewer-paused-landscape.png)
- [web-landing.png](ui/multies/web-landing.png)
- [web-member-390.png](ui/multies/web-member-390.png)
- [web-recovery-large-top.png](ui/multies/web-recovery-large-top.png)
- [web-approved-large-status.png](ui/multies/web-approved-large-status.png)
- [web-operator-320-top.png](ui/multies/web-operator-320-top.png)
- [web-policy-large-action.png](ui/multies/web-policy-large-action.png)
- [web-request-large-action.png](ui/multies/web-request-large-action.png)

## Independent local form UX follow-up

Connection fields now have explicit keyboard Next order, with Done finishing editing rather than starting a connection. On supported Android versions the auth-key field requests no personalized keyboard learning. An incomplete form focuses the missing field and immediately reveals it with its persistent label; the previous animated scroll could leave that label outside the captured viewport. Existing Review & connect, disclosure and OS consent remain required. Framework tests verify keyboard order and that Done/invalid setup do not save join details or start the VPN. This does not verify an installed keyboard or TalkBack.

New required-field fixtures were rendered at390px,320px/enlarged text and740px landscape. Actual field/label pixels were inspected; native error-popup text is not part of this host capture and is not claimed as accepted. Nine layout tests pass with60 synthetic screenshots; the selected framework suite now has176 passing cases.

## Independent local web keyboard/recovery follow-up

The operator sign-in previously described every request failure as a wrong password, allowed another submission while pending, and left keyboard focus in hidden controls across sign-in/sign-out. The local UI now distinguishes a rejected password from an unavailable request, guards pending submission, associates the status message with the password field, marks only rejected/empty password input invalid, and restores focus to visible login/overview controls. A refresh of an already-open dashboard does not steal focus from an in-progress field.

Isolated real Chrome tests cover empty/rejected login, held request plus Enter,503 recovery, keyboard retry,320px/200-percent text, initial/entered/signed-out focus and refresh focus retention, alongside the prior dashboard regressions. The inspected enlarged retry screenshot shows the focus ring, label, retry action and readable error. This is browser DOM/pixel evidence, not screen-reader acceptance. Fresh control race tests and separate account/provider browser regressions pass. No server permission/session policy changed.

The member account received the same bounded keyboard recovery review. Whitespace-only network IDs focus their field before any request; a pending sign-in cannot be submitted twice. Failed sign-in preserves the entered ID, restores field focus and displays an associated inline error beside the retry action rather than only a message above the viewport. Entering the device list or returning to sign-in uses visible focus, ordinary refresh preserves it, and an uncertain sign-out focuses its retry action without restoring cleared private rows. Current Chrome fixtures verify these paths, including320px/200-percent retry pixels. The inspected inline-error/retry screenshot includes the full message and focused action together. Existing provider/ownership/logout-replay/late-response checks still pass.

## Supported-appearance boundary

Read-only source review confirms Multies itself supports system/light/dark and blue/Forest. Mnet has no appearance setting or shared workspace preference and currently implements only Multies light/blue: explicit web color-scheme, Android light platform theme/fixed colors, SwiftUI fixed light colors. This checkpoint does not add or claim full Multies appearance integration.

Chrome now emulates an actual dark system preference and checks that the supported Mnet light palette/native fields remain consistent; member and operator screenshots were inspected. Android explicitly opts its custom view tree out of automatic Force Dark recoloring. An API33 night-qualified actual-resource fixture verifies the opt-out and rerenders the supported light home/settings/error states; home and missing-field pixels were inspected. Ten layout tests pass with67 synthetic PNGs. This checks behavior under a dark preference, not dark-theme implementation or installed OS behavior.

Swift source now explicitly requests light appearance for its fixed-light panel and navigation rather than mixing fixed-light surfaces with inherited native dark controls. This correction is source-only and has not been compiled or visually accepted. Current Swift compiler/native pixels remain required. Native OS/system alerts are outside the inspected view raster. No workspace theme, accent picker, auth/consent behavior or appearance setting was added.

## ACL editor recovery and draft safety

The existing editor could silently replace a draft on the ten-second dashboard refresh, reset its network selection, or leave old-network rules visible after a failed read. The local UI now preserves the selected target/draft on refresh, clears and disables unread policy text, rejects malformed reads, suppresses obsolete read results and asks before discarding edits on explicit reload/network switching. Saving locks its target while pending and prevents duplicate submissions. JSON parse failures, server rejection and an uncertain/lost save response are separate states; an uncertain result requires stored-policy inspection before retry. Sign-out clears the editor and invalidates late results. Policy semantics and backend permissions are unchanged.

Isolated Chrome tests use disposable networks only. They cover null/empty policy reads, invalid JSON, malformed/failed reads, refresh preservation, cancelled reload/switch, target locking, server rejection, a real committed save with its response dropped followed by inspection, out-of-order reads, switching and a late save result after logout. Fresh full Go race tests, vet and the separate account/provider browser regression pass. A320px/200-percent error/reload screenshot was inspected after fixing spacing and long-network-ID wrapping. This is local source/browser evidence, not a live policy update.

The first expanded browser attempt stalled because a test awaited its intentionally held read before releasing it; that fixture was corrected and rerun. An immediate-after-switch test also raced a fast successful local response and was corrected to hold the read while asserting the loading state. Neither failure was counted as passing acceptance.

The adjacent auth-key creation form had the same refresh target-reset issue: its network choice now survives refresh alongside the existing note draft. Chrome verifies the selected target and note remain unchanged. Repeated row actions now have contextual accessible names including device/address, key note/network or membership principal/issuer. Names are assigned with DOM attributes, not HTML interpolation. Real Chrome accessibility-tree checks verify the resulting button names using synthetic populated rows, including markup-like text that remains inert. This checks browser accessibility metadata, not screen-reader acceptance; no visible controls or action permissions were removed.

The table review also exposed keyboard focus being detached by periodic row replacement. Rendering now restores focus to the same stable row action without moving the viewport, or to its table if the row disappeared. Chrome verifies both states. The new narrow-screen action screenshot initially exposed words fragmenting inside undersized action cells, then a clipped outer focus ring. Action cells now keep button labels whole with spacing and horizontal scroll clearance. The corrected390px pixels were inspected: Expire key and Remove are readable and the focused Remove ring stays inside the viewport. This is synthetic row/layout evidence, not a real device removal.

## Async follow-up review

A regression added before the fix demonstrated that background dashboard refresh could clear the new uncertain-policy-save state and re-enable Save without explicit inspection. The editor now records which selection has already been read independently from whether a valid writable target exists. Routine refresh leaves failed/uncertain/loading states intact; explicit Reload still inspects the stored policy. Dashboard state reads now also use request ordering so an older response cannot replace newer rendered state. Isolated Chrome tests verify both cases, including a dropped committed policy-save response and reversed state responses. These are local UI corrections, with no backend permission or policy-semantic change.

## Network/auth-key creation recovery

Both creation forms previously allowed repeat submissions and reported failures through blocking alerts; auth-key success used a separate prompt even though the key was already in the table. The forms now lock submitted fields/target while pending, keep inline status, preserve drafts after an explicit4xx rejection, and keep ambiguous network/5xx outcomes unconfirmed instead of encouraging duplicate creation. An Inspect creation result action refreshes and focuses the relevant table without clearing the uncertainty guard. Reloading the page is an explicit follow-up after inspection, not an automatic write retry. Success points to the existing table rather than duplicating a key in a dialog. Key values are never placed in status/accessibility text.

Network names reject whitespace locally. Key lifetime input no longer silently truncates fractions through parseInt; it requires a safe whole number of hours0 or more. Confirmed success clears submitted draft text; sign-out clears creation drafts and outcomes. Chrome tests cover held/double submissions, explicit rejections, real disposable creations with lost responses, direct inspection, duplicate guards surviving refresh, draft retention/cleanup and enlarged outcome pixels. Fresh full race tests, vet and separate account/provider browser regressions pass. The320px/200-percent outcome/inspection screenshot was inspected. No user account or live network/key was changed.

## Native viewer message recovery

Viewer chat previously cleared a draft before its send result and silently swallowed errors. It now keeps drafts on failed/uncertain sends, prevents duplicate pending sends, preserves a newer draft when an older send completes, and suppresses stale completion after viewing ends/reconnect invalidation. Queue acceptance is explicitly not a delivery confirmation. The input has a persistent associated Message label and a polite status region; no message text is copied into status. Session/transport/control authority is unchanged and there is no automatic resend.

Fresh184 framework simulations pass across API24/25/26/33, including held/failing sends, duplicate prevention, newer draft preservation and late completion after End viewing. Ten actual-resource/API33 raster fixtures pass with70 synthetic PNGs. Failed-send pixels were inspected at390px,320px/enlarged text and740px landscape: retained draft, recovery message, Send and persistent End viewing remain readable/reachable. This is fake-transport UI evidence, not delivery, real keyboard or TalkBack acceptance.

## Native button focus treatment

Custom Multies-colored button drawables had replaced the native background without a focused state. Both primary and secondary controls now have a visible source-palette focus treatment; disabled/pressed state precedence and click behavior stay unchanged. Actual focused home and secondary-action pixels were inspected at390px, enlarged320px and landscape. Ten raster tests pass with78 synthetic PNGs;188 framework cases pass across API24/25/26/33, including distinct focused drawable states and focus not triggering an action. The capture requests focus directly, so this does not establish physical D-pad/keyboard or TalkBack acceptance.

## Browser forced-colors boundary

Actual Chrome forced-colors emulation revealed that fill-only buttons lost their visible boundaries. A scoped forced-colors rule now uses browser/system ButtonText, GrayText and Highlight values for borders/focus rather than overriding the person's contrast palette. Default Multies light/blue rendering is unchanged. Member/operator sign-in and enlarged policy-reload states were rerendered and inspected after the fix; action boundaries, focus and disabled-versus-enabled controls remain visible. Both real browser regressions pass with explicit media/state/border assertions. This is browser emulation, not acceptance of every Windows high-contrast palette or assistive technology.

## Member read recovery

A device-list service/network/invalid-response failure no longer implies that the browser is signed out. Stale private rows are cleared, while a separate recovery panel offers a same-session read retry and Sign out. Only explicit unauthorized/forbidden responses use the sign-in view. The recovery heading receives focus; successful retry restores the account heading. Real Chrome tests cover malformed rows, 503, connection failure, direct same-session retry and failed sign-out via this new panel, alongside the existing out-of-order/logout privacy guards. Inspected actual 390px and 320px enlarged renders; current control race/browser tests pass. No identity-provider login, membership or session creation behavior changed.

## Operator device and auth-key actions

Approval, denial/removal, key expiry and auth-key deletion now review the current target and consequence before sending. A shared inline outcome panel sits above the relevant table. Row actions lock across pending/approved/key tables while a request is unresolved, including after rerender. Explicit rejection is separate from unknown outcome; unknown outcomes require a successful explicit inspection before new row actions. Inspection only reads. Background refresh does not bypass that lock. Late completions after sign-out cannot restore feedback or private rows. A failed sign-out generation change leaves the unresolved action inspectable once it settles.

Real Chrome tests cover all four operations' cancellation, contextual review, stale confirmation, pending duplicate/rerender guards and rejection; uncertain expiry; failed inspection and background refresh; and a real disposable auth-key deletion whose committed response was deliberately dropped, followed by read-only inspection with exactly one mutation. Native dialog handling is simulated for deterministic acceptance/cancellation, not a claim about browser/OS dialog styling. A test initially assumed an empty key list was an array rather than nullable JSON, and was corrected to match the existing API. Inspected 390px and 320px/200% outcome/retry pixels. Production resources were not used.

## Dashboard refresh honesty

A failed initial state read now has a visible retry/sign-out route instead of a blank app. A failed latest refresh visibly marks retained data as potentially out of date and pauses dashboard controls until a valid refresh succeeds, without clearing drafts. Header Refresh/Sign out and the explicit retry remain usable. Payload shape is checked before replacing displayed state, including nullable collections and boolean presence data. Existing per-action pending/unknown locks remain independent and are not cleared by that refresh. The new panel's focused Retry returns keyboard focus to the overview when it succeeds. Older failed reads cannot override newer successful reads.

Real browser coverage includes initial 503, connection failure, seven malformed collection/row/presence payloads, retained network/policy drafts, disabled-control and direct-create guards, successful retry/focus and reverse-completion failures. Actual 390px initial failure and 320px/200% stale-data pixels were inspected. The enlarged sticky notice leaves its retry visible; horizontal device tables retain their existing separate scroll behavior.

## Literal identifiers in operator controls

Row actions now attach DOM event listeners with exact identifier values rather than building inline JavaScript strings. Pending-row IDs and network options are assigned through DOM properties/constructors. Text escaping also covers quotes. The browser regression uses synthetic quote/markup-like identifiers to verify all five row buttons receive the exact target, no inline event attributes/extra attributes appear, no script runs and option/row values stay literal. Existing contextual names, focus restoration and mutation recovery tests still pass. This is preventive boundary testing, not evidence of an attack against production.

## Narrow-screen operator records

Device, pending-approval and auth-key tables now show labeled vertical records at widths up to 600px, keeping every existing field and action visible without sideways hunting. Desktop retains the table layout. Mobile action buttons wrap as needed for enlarged text, and existing stable focus/uncertain-action behavior is retained. Missing user/activity values read Not recorded; empty endpoint lists read None reported rather than unlabeled gaps. A missing/zero-year activity timestamp is not shown as a very old real observation. Boolean presence checks do not treat inherited object properties as connected devices.

Inspected actual 390px device, pending-approval and auth-key records, a complete 320px/200% device record, the 1280px desktop table and the refreshed uncertain-action panel. Device identity no longer breaks into a column of short fragments. Both enlarged action buttons remain in the same vertical record. Explicit table/rowgroup/row/cell/header roles and real column headers are preserved; Chrome's accessibility tree exposes the three table names at mobile width. This is not a screen-reader acceptance claim. Browser coverage checks no side overflow for the three record tables, mobile field labels, enlarged controls, preserved desktop layout, contextual names and focus. The remaining membership table still supports keyboard horizontal scroll and has its own fixture. Current control race/vet and brand/diff checks pass, including an added concurrent failed-sign-out/action-settlement recovery check.

## Final member sign-out review

Pixel review caught an incomplete recovery state: an unconfirmed sign-out still displayed an empty approved-devices section with a blank network ID. It now has its own Sign-out not confirmed panel with focused Retry sign-out and a separate read-only Check this session. An explicit session read can recover without a new sign-in. Private rows stay cleared. The real browser tests use the visible retry for both a failed logout and a logout committed with its response lost, checking token replay denial and idempotent retry. Inspected 390px and 320px/200% recovery pixels. Row-action inspection copy also retains its target context; auth-key review includes its non-secret ID, never its secret value.

## Member record readability

The final 320px/200% member-table inspection found the same cramped-column problem: personal-device names and mesh addresses broke into short fragments. The two-field member table now uses labeled vertical records below 600px, retaining the desktop table and explicit table/row/cell semantics. Actual enlarged pixels now show each full mesh address on one line, with the complete device name above it. Browser tests check field-label visibility, no sideways overflow and restoration of the desktop layout, along with all sign-in/read/sign-out privacy/recovery cases. Current member browser and control race/vet/diff checks pass. Existing native host-bottom, viewer-frame and landscape-control images were inspected during this final pass; no new native source change or installed-device acceptance is claimed.

## Native status-to-details route

Android home approval/error guidance previously referred to details below the session and manual-settings sections without a direct route. After a status arrives, the connection card now offers View approval details, View error details or View connection details. It only scrolls to and focuses the existing details heading. It never opens the approval URL, starts a service/activity, changes the connection or adds permissions. Initial no-status layout does not show the extra action.

Fresh192 framework cases pass across API24/25/26/33, including action labels, heading focus, no service/activity launch and unchanged connection state. Ten packaged-resource raster tests pass with90 synthetic images. Both approval and error jumps assert visible heading bounds at390px,320px/160% text,740px landscape and system-dark/light-palette configurations. Inspected actual enlarged approval/error route pixels,390px home status and landscape details pixels. End-of-page scroll clamping can leave preceding settings visible, but the heading and existing recovery/approval content remain on screen. This is simulated Android layout, not installed keyboard/TalkBack, real permission or VPN acceptance. The initial command lacked the restored Maven path after pause and did not run tests; rerunning with the existing JDK/Maven environment produced the passing results above.

## Publication verification

The existing Mnet Pages source remains main:/docs. Build44477501897 succeeded for76978fbccc7631ff9109153aa0ba20dacc14fe29. The Markdown checkpoint and selected approval/member/operator previews were fetched from the deployed site and matched the committed bytes; deployed approval/member pixels were inspected. This site serves engineering Markdown directly rather than generating an HTML route. The privacy-policy landing was not edited. The closed combined Android emulator workflow was cancelled; normal CI continued. No app release, signing, production-app deployment or Multies mutation occurred.

## Remote current-source check result

Normal CI [44477503883](https://github.com/MaisySen/Mnet/actions/runs/44477503883) completed successfully for source commit76978fbccc7631ff9109153aa0ba20dacc14fe29. All five jobs passed: Go/browser/framework/layout checks, Windows, macOS, session validation and the existing kernel fixture. macOS includes the iOS SDK source check, shared hosted SwiftUI pixels and unsigned simulator app/extension linkage. No simulator launch or installed-device appearance, input, accessibility, VPN or screen-control result is inferred from that build. Hosted SwiftUI evidence above remains bounded to macOS component rendering. This follow-up only publishes documentation and the inspected synthetic Swift images, not new application behavior.

## Membership pending/recovery follow-through

A new failing Chrome regression demonstrated that account-membership grant submission cleared its verified-principal draft before the outcome and left target/form/read controls unlocked. A single scoped operation now locks both grants and removals while pending. Duplicate submissions, direct read calls and ordinary dashboard refresh cannot replace the operation. An explicit4xx rejection restores the issuer/subject draft and requires verification again. A transport/5xx result remains unconfirmed, with changes locked until a successful read-only inspection; failed inspection does not unlock it. Exact principal review and backend membership semantics are unchanged.

A sign-out attempt clears private rows/drafts immediately. If sign-out fails during a pending membership write, the result becomes unconfirmed and the inspection route remains, while late old-generation completion cannot repaint the draft or clear that guard. Existing successful sign-out, network-switch, out-of-order-read and malformed-response checks remain passing. The real local commit/lost-response test proves inspection does not resend the membership write. Held/rejected removal, rejected grant draft, pending read/refresh/duplicate guards and failed inspection are covered in real Chrome; all resources are disposable local fixtures.

Inspected [320px/200% failed-inspection recovery](ui/multies/membership-unconfirmed-320.png). The first pixel review showed Inspect grants too far above its outcome, so it now follows the status directly. The grant remains disabled beside the unresolved notice; Inspect remains available. This is browser pixel/DOM evidence, not screen-reader acceptance. Current control race/vet, expanded Chrome and brand/diff checks pass. No production membership, Multies or app release changed.

## Operator sign-out recovery

Operator sign-out failure previously used a transient alert containing server error text, with no persistent retry outcome. It now displays a focused Sign-out was not confirmed status and Retry sign-out action outside the dashboard controls. Pending sign-out locks both routes and dashboard changes, with a direct guard against duplicate calls. Unknown results do not claim the session ended. Existing mutation-specific inspection guards remain independent.

Real Chrome covers a503 recovery, held direct retry/duplicate guard, fixed non-technical error copy, and an actual server logout committed before its response is dropped. Retrying that committed logout receives401 and correctly reports the session is no longer active, rather than claiming a new successful logout. The initial expanded test expected a second success and failed, exposing that distinction; the UI and assertion now follow the real server behavior without changing the backend. The replayed old token is denied; delayed rows/policy/membership state cannot restore signed-out data. A separate fresh login/ordinary successful sign-out still returns focused sign-in and clears the warning. The first screenshot run had no output directory and did not count as acceptance; the directory was created and the suite rerun.

Inspected [320px/200% persistent sign-out retry](ui/multies/operator-signout-unconfirmed-320.png): the full fixed message, focus ring and direct retry fit. Expanded browser, current control race/vet and brand/diff checks pass. No production session was used and no session policy, membership permission or backend route changed.

## Narrow membership records

The remaining membership table separated private issuer/subject identifiers from their action through sideways scrolling. At600px and below it now keeps the issuer, subject and removal action in one labelled record, preserving desktop tables and explicit table/header/cell roles. Identifiers remain literal text and the removal confirmation still shows the exact principal and network. The first enlarged capture fragmented the long Remove membership label, so the visible label is now Remove grant while its contextual accessible name keeps full membership/identity scope.

Inspected [390px record](ui/multies/membership-record-390.png) and [320px/200% record](ui/multies/membership-record-320.png): both complete identifiers and the action are reachable without sideways hunting, and action words stay intact. Chrome checks compact/enlarged overflow, mobile table/header semantics, desktop restoration and existing mutation guards. Current control race/vet and expanded Chrome pass. This remains browser evidence, not screen-reader acceptance. The keyboard horizontal-scroll regression now targets the remaining Networks table rather than the record-layout membership table.

## Membership and operator-recovery remote verification

Normal CI completed successfully for each application checkpoint: [membership operation guards44486236024](https://github.com/MaisySen/Mnet/actions/runs/44486236024), [operator sign-out44486808146](https://github.com/MaisySen/Mnet/actions/runs/44486808146), and [responsive membership records44487100579](https://github.com/MaisySen/Mnet/actions/runs/44487100579). All five jobs passed in each. Latest application source is91b38942fe5604872029d700fefa12e3cb74ce91. The existing Pages build44487099683 succeeded; deployed Markdown and current membership-record/operator-signout PNGs matched committed bytes and the downloaded pixels were inspected.

Real Chrome Tab events additionally verify that focused membership outcome text leads to the enabled Inspect grants action, and focused sign-out outcome leads to Retry sign-out, both at320px/200% text with at least44px action height. This is browser keyboard evidence, not screen-reader or installed-device acceptance. No additional product change was inferred from that bounded review. Multies, releases, production sessions/grants and native execution scope remain unchanged.
