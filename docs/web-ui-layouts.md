# Web member and operator layouts

Updated 19 November 2026, Australia/Sydney. These are implemented Go-served pages, captured in an isolated real Chrome browser against local disposable control/store fixtures. Member sign-in uses a local test identity provider. No user account, production deployment, Multies change or app release is implied.

The same gray-green canvas, white cards, dark type and teal actions now connect the landing page, member account, sign-in recovery and operator dashboard to the Android visual system. Member and operator paths remain separate. Device approval, account membership and current connectivity retain their existing meaning. The legal/provider identity text is unchanged.

## Inspected pixels

- [Landing](ui/web/account-landing.png): separate member/operator entry cards and explicit development/manual-setup limits.
- [Member mobile,390px](ui/web/account-mobile.png): approved devices, readable mesh addresses and stacked Refresh/Sign out actions.
- [Empty member list](ui/web/account-empty.png): empty is not confused with failure or online status.
- [Sign-in recovery](ui/web/account-login-failed.png), [320px/200% text](ui/web/account-recovery-320-top.png): no provider errors or private callback parameters echoed; real return-to-sign-in route.
- [Member lower actions,320px/200% text](ui/web/account-320-large-bottom.png): scroll-reachable Refresh and Sign out; browser-session/VPN distinction remains visible.
- [Operator sign-in](ui/web/dashboard-login.png), [overview](ui/web/dashboard-overview.png), [320px top](ui/web/dashboard-320-top.png): persistent labels, larger controls and section navigation.
- [Keyboard-focused table far edge,320px](ui/web/dashboard-320-table-end.png): table overflow stays within the card and has keyboard focus. ArrowRight is tested as actual browser input; the final far-edge screenshot uses scrollLeft, not a claimed End-key shortcut.
- [Policy edit then save,320px/200% text](ui/web/dashboard-320-large-policy-action.png): Save policy follows the editor with spacing. Existing API/authorization behavior is unchanged.

Images with top/overview in their filename are crops of actual browser output. The lower-action images are actual320x720 viewport captures after scrolling. Others are full-page captures. Random network IDs and device names belong to disposable local fixtures, not a real customer's network. Root font-size200% checks enlarged text, not every browser/OS zoom or accessibility combination.

## Reproduce

Chrome (or CHROME_BIN), Node22 and Go are needed. Output directories must exist.

```sh
mkdir -p /tmp/mnet-web-account /tmp/mnet-web-admin
MNET_ACCOUNT_SHOTS=/tmp/mnet-web-account MNET_ACCOUNT_BROWSER=1 go test -race -count=1 -timeout=2m ./internal/control -run '^TestAccountRealChrome$'
MNET_DASHBOARD_SHOTS=/tmp/mnet-web-admin node scripts/dashboard-validation.mjs
go test -race -count=1 ./...
go vet ./...
sh scripts/check-secrets.sh
```

The existing browser regressions retain real sign-in/logout, identity-filtered account reads, empty/error/retry handling, stale/lost responses, reviewed manual grants, removal, HTML-as-text and session-replay checks. New geometry assertions check320px layouts,200% root text, lower action placement, table keyboard scrolling and policy edit/save order. They are regression coverage, not full accessibility certification. Initial enlarged-text failures exposed header/heading overflow and were fixed before these captures. End did not scroll the table horizontally; ArrowRight is the tested key.

This checkpoint does not cover native phone/browser integration, physical-device networking, new Multies enrollment, all assistive technologies or production load. Existing signing, release and native-control gates remain separate. The older browser approval surface is not redesigned by this checkpoint.
