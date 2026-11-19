# Account sign-in response ownership

This is a source-only correction to the existing Mnet engineering account page. It does not change Multies, the identity provider, membership, enrollment, design, legal text or a hosted control-service deployment.

## Reproduced behavior

A delayed sign-in response could navigate the browser after an intervening session recheck had shown the authenticated account and the user had signed out. The page already guarded late device reads, but its sign-in request did not have equivalent ownership checks.

An isolated real-Chrome regression reproduced this against the old source: a held sign-in response navigated to a retired-response marker after confirmed sign-out. The browser fixture uses disposable providers, local stores and synthetic devices, not a live account.

## Correction

Each sign-in now owns a generation and an attempt number. Both the HTTP response and its asynchronous JSON body must still belong to that generation and attempt before navigation. A retired error cannot replace current feedback or move focus. Sign-out retires the old attempt and makes retry available. An older request's cleanup cannot re-enable the button or clear the in-progress flag while a newer request is pending.

The new Chrome cases cover delayed response headers, delayed JSON and a rejected request. Each starts a newer attempt before completing the retired one, checks that current feedback, field contents and pending-button ownership stay intact, then verifies retry and a fresh real-provider fixture login.

## Evidence and limits

The complete real-Chrome account journey passes three repetitions. Existing private-device clearing, lost sign-out responses, keyboard retry, 320px/200% text, narrow device records and forced-color checks remain included. Actual screenshots of the desktop late-response retry, signed-out page and narrow enlarged-text retry were inspected. They keep the existing recovery text and action readable; no layout/style change was needed.

Full repository race tests, vet and tracked-source checks pass locally. Hosted CI and deployment checks are separate. Browser pixels and a disposable provider do not prove screen-reader behavior, native authentication, installed-app behavior or live Multies integration.

```sh
MNET_ACCOUNT_BROWSER=1 go test -race -count=3 -timeout=2m \
  ./internal/control -run '^TestAccountRealChrome$'
go test -race -count=1 -timeout=5m ./...
go vet ./...
sh scripts/check-secrets.sh
```
